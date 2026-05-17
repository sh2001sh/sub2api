package provider

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
)

const (
	xunhuPayVersion         = "1.1"
	xunhuPayDefaultAPIBase  = "https://api.xunhupay.com/payment"
	xunhuPayHTTPTimeout     = 15 * time.Second
	xunhuPayMaxResponseSize = 1 << 20

	xunhuPayStatusPaid          = "OD"
	xunhuPayStatusPending       = "WP"
	xunhuPayStatusCancelled     = "CD"
	xunhuPayRefundedStatus      = "CD"
	xunhuPayRefundPendingStatus = "RD"
	xunhuPayRefundFailedStatus  = "UD"
)

// XunhuPay implements payment.Provider for 虎皮椒支付 / 迅虎支付.
type XunhuPay struct {
	instanceID string
	config     map[string]string
	httpClient *http.Client
}

// NewXunhuPay creates a new XunhuPay provider instance.
func NewXunhuPay(instanceID string, config map[string]string) (*XunhuPay, error) {
	for _, key := range []string{"appId", "appSecret", "notifyUrl", "returnUrl"} {
		if strings.TrimSpace(config[key]) == "" {
			return nil, fmt.Errorf("xunhupay config missing required key: %s", key)
		}
	}

	cfg := make(map[string]string, len(config)+1)
	for key, value := range config {
		cfg[key] = value
	}
	cfg["apiBase"] = normalizeXunhuPayAPIBase(cfg["apiBase"])
	if cfg["apiBase"] == "" {
		cfg["apiBase"] = xunhuPayDefaultAPIBase
	}

	return &XunhuPay{
		instanceID: instanceID,
		config:     cfg,
		httpClient: &http.Client{Timeout: xunhuPayHTTPTimeout},
	}, nil
}

func normalizeXunhuPayAPIBase(raw string) string {
	base := strings.TrimSpace(raw)
	if base == "" {
		return ""
	}
	if parsed, err := url.Parse(base); err == nil && parsed.Scheme != "" && parsed.Host != "" {
		parsed.RawQuery = ""
		parsed.Fragment = ""
		parsed.RawPath = ""
		parsed.Path = trimXunhuPayEndpointPath(parsed.Path)
		return strings.TrimRight(parsed.String(), "/")
	}
	return strings.TrimRight(trimXunhuPayEndpointPath(base), "/")
}

func trimXunhuPayEndpointPath(path string) string {
	path = strings.TrimRight(strings.TrimSpace(path), "/")
	lower := strings.ToLower(path)
	for _, endpoint := range []string{"/do.html", "/query.html", "/refund.html"} {
		if strings.HasSuffix(lower, endpoint) {
			return strings.TrimRight(path[:len(path)-len(endpoint)], "/")
		}
	}
	return path
}

func (x *XunhuPay) apiBase() string {
	if x == nil {
		return ""
	}
	return normalizeXunhuPayAPIBase(x.config["apiBase"])
}

func (x *XunhuPay) Name() string        { return "XunhuPay" }
func (x *XunhuPay) ProviderKey() string { return payment.TypeXunhuPay }
func (x *XunhuPay) SupportedTypes() []payment.PaymentType {
	return []payment.PaymentType{payment.TypeAlipay, payment.TypeWxpay}
}

func (x *XunhuPay) MerchantIdentityMetadata() map[string]string {
	if x == nil {
		return nil
	}
	appID := strings.TrimSpace(x.config["appId"])
	if appID == "" {
		return nil
	}
	return map[string]string{"appid": appID}
}

func (x *XunhuPay) CreatePayment(ctx context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	notifyURL, returnURL := x.resolveURLs(req)
	payload := map[string]string{
		"version":        xunhuPayVersion,
		"appid":          x.config["appId"],
		"trade_order_id": req.OrderID,
		"total_fee":      req.Amount,
		"title":          req.Subject,
		"notify_url":     notifyURL,
		"return_url":     returnURL,
		"callback_url":   returnURL,
		"time":           strconv.FormatInt(time.Now().Unix(), 10),
		"nonce_str":      xunhuPayNonce(),
	}
	payload["hash"] = xunhuPaySign(payload, x.config["appSecret"])

	var resp struct {
		ErrCode   any    `json:"errcode"`
		ErrMsg    string `json:"errmsg"`
		URL       string `json:"url"`
		URLQRCode string `json:"url_qrcode"`
		OrderID   string `json:"order_id"`
	}
	if err := x.postJSON(ctx, x.apiBase()+"/do.html", payload, &resp); err != nil {
		return nil, fmt.Errorf("xunhupay create payment: %w", err)
	}
	if !xunhuPayCodeIsSuccess(resp.ErrCode) {
		return nil, fmt.Errorf("xunhupay create payment failed: %s", strings.TrimSpace(resp.ErrMsg))
	}

	payURL := strings.TrimSpace(resp.URL)
	qrCode := strings.TrimSpace(resp.URLQRCode)
	if qrCode == "" {
		qrCode = payURL
	}
	if req.IsMobile && payURL == "" {
		payURL = qrCode
	}

	return &payment.CreatePaymentResponse{
		TradeNo: strings.TrimSpace(resp.OrderID),
		PayURL:  payURL,
		QRCode:  qrCode,
	}, nil
}

func (x *XunhuPay) QueryOrder(ctx context.Context, tradeNo string) (*payment.QueryOrderResponse, error) {
	payload := map[string]string{
		"version":         xunhuPayVersion,
		"appid":           x.config["appId"],
		"out_trade_order": tradeNo,
		"time":            strconv.FormatInt(time.Now().Unix(), 10),
		"nonce_str":       xunhuPayNonce(),
	}
	payload["hash"] = xunhuPaySign(payload, x.config["appSecret"])

	var resp struct {
		ErrCode any    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
		Data    struct {
			TotalFee      string `json:"total_fee"`
			Status        string `json:"status"`
			TradeOrderID  string `json:"trade_order_id"`
			TransactionID string `json:"transaction_id"`
			AppID         string `json:"appid"`
		} `json:"data"`
	}
	if err := x.postJSON(ctx, x.apiBase()+"/query.html", payload, &resp); err != nil {
		return nil, fmt.Errorf("xunhupay query order: %w", err)
	}
	if !xunhuPayCodeIsSuccess(resp.ErrCode) {
		return nil, fmt.Errorf("xunhupay query order failed: %s", strings.TrimSpace(resp.ErrMsg))
	}

	amount, _ := strconv.ParseFloat(strings.TrimSpace(resp.Data.TotalFee), 64)
	status := payment.ProviderStatusPending
	switch strings.TrimSpace(resp.Data.Status) {
	case xunhuPayStatusPaid:
		status = payment.ProviderStatusPaid
	case xunhuPayStatusCancelled:
		status = payment.ProviderStatusFailed
	case xunhuPayStatusPending:
		status = payment.ProviderStatusPending
	}

	metadata := x.MerchantIdentityMetadata()
	if strings.TrimSpace(resp.Data.AppID) != "" {
		if metadata == nil {
			metadata = map[string]string{}
		}
		metadata["appid"] = strings.TrimSpace(resp.Data.AppID)
	}
	if strings.TrimSpace(resp.Data.Status) != "" {
		if metadata == nil {
			metadata = map[string]string{}
		}
		metadata["status"] = strings.TrimSpace(resp.Data.Status)
	}

	tradeID := strings.TrimSpace(resp.Data.TransactionID)
	if tradeID == "" {
		tradeID = tradeNo
	}
	return &payment.QueryOrderResponse{
		TradeNo:  tradeID,
		Status:   status,
		Amount:   amount,
		Metadata: metadata,
	}, nil
}

func (x *XunhuPay) VerifyNotification(_ context.Context, rawBody string, _ map[string]string) (*payment.PaymentNotification, error) {
	values, err := url.ParseQuery(rawBody)
	if err != nil {
		return nil, fmt.Errorf("xunhupay parse notification: %w", err)
	}

	params := make(map[string]string, len(values))
	for key := range values {
		params[key] = values.Get(key)
	}
	hash := strings.TrimSpace(params["hash"])
	if hash == "" {
		return nil, fmt.Errorf("xunhupay notification missing hash")
	}
	if !strings.EqualFold(hash, xunhuPaySign(params, x.config["appSecret"])) {
		return nil, fmt.Errorf("xunhupay notification invalid hash")
	}

	if strings.TrimSpace(params["status"]) != xunhuPayStatusPaid {
		return nil, nil
	}

	amount, _ := strconv.ParseFloat(strings.TrimSpace(params["total_fee"]), 64)
	metadata := x.MerchantIdentityMetadata()
	if appID := strings.TrimSpace(params["appid"]); appID != "" {
		if metadata == nil {
			metadata = map[string]string{}
		}
		metadata["appid"] = appID
	}
	metadata["status"] = strings.TrimSpace(params["status"])

	return &payment.PaymentNotification{
		TradeNo:  strings.TrimSpace(params["transaction_id"]),
		OrderID:  strings.TrimSpace(params["trade_order_id"]),
		Amount:   amount,
		Status:   payment.ProviderStatusSuccess,
		RawData:  rawBody,
		Metadata: metadata,
	}, nil
}

func (x *XunhuPay) Refund(ctx context.Context, req payment.RefundRequest) (*payment.RefundResponse, error) {
	payload := map[string]string{
		"version":        xunhuPayVersion,
		"appid":          x.config["appId"],
		"trade_order_id": req.OrderID,
		"refund_fee":     req.Amount,
		"reason":         req.Reason,
		"time":           strconv.FormatInt(time.Now().Unix(), 10),
		"nonce_str":      xunhuPayNonce(),
	}
	payload["hash"] = xunhuPaySign(payload, x.config["appSecret"])

	var resp struct {
		ErrCode       any    `json:"errcode"`
		ErrMsg        string `json:"errmsg"`
		RefundOrderID string `json:"refund_order_id"`
		RefundStatus  string `json:"refund_status"`
	}
	if err := x.postJSON(ctx, x.apiBase()+"/refund.html", payload, &resp); err != nil {
		return nil, fmt.Errorf("xunhupay refund: %w", err)
	}
	if !xunhuPayCodeIsSuccess(resp.ErrCode) {
		return nil, fmt.Errorf("xunhupay refund failed: %s", strings.TrimSpace(resp.ErrMsg))
	}

	status := payment.ProviderStatusPending
	switch strings.TrimSpace(resp.RefundStatus) {
	case xunhuPayRefundedStatus:
		status = payment.ProviderStatusSuccess
	case xunhuPayRefundPendingStatus:
		status = payment.ProviderStatusPending
	case xunhuPayRefundFailedStatus:
		status = payment.ProviderStatusFailed
	}

	refundID := strings.TrimSpace(resp.RefundOrderID)
	if refundID == "" {
		refundID = strings.TrimSpace(req.OrderID) + "-refund"
	}
	return &payment.RefundResponse{RefundID: refundID, Status: status}, nil
}

func (x *XunhuPay) resolveURLs(req payment.CreatePaymentRequest) (string, string) {
	notifyURL := strings.TrimSpace(req.NotifyURL)
	if notifyURL == "" {
		notifyURL = strings.TrimSpace(x.config["notifyUrl"])
	}
	returnURL := strings.TrimSpace(req.ReturnURL)
	if returnURL == "" {
		returnURL = strings.TrimSpace(x.config["returnUrl"])
	}
	return notifyURL, returnURL
}

func (x *XunhuPay) postJSON(ctx context.Context, endpoint string, payload any, dest any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := x.httpClient
	if client == nil {
		client = &http.Client{Timeout: xunhuPayHTTPTimeout}
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, xunhuPayMaxResponseSize))
	if err != nil {
		return err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	if dest == nil {
		return nil
	}
	if err := json.Unmarshal(respBody, dest); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}
	return nil
}

func xunhuPayCodeIsSuccess(code any) bool {
	switch typed := code.(type) {
	case float64:
		return int(typed) == 0
	case int:
		return typed == 0
	case string:
		typed = strings.TrimSpace(typed)
		return typed == "0" || typed == ""
	default:
		return false
	}
}

func xunhuPayNonce() string {
	sum := md5.Sum([]byte(strconv.FormatInt(time.Now().UnixNano(), 10)))
	return hex.EncodeToString(sum[:8])
}

func xunhuPaySign(params map[string]string, secret string) string {
	keys := make([]string, 0, len(params))
	for key, value := range params {
		if strings.EqualFold(key, "hash") || strings.TrimSpace(value) == "" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys)+1)
	for _, key := range keys {
		parts = append(parts, key+"="+params[key])
	}
	parts = append(parts, "appsecret="+secret)
	sum := md5.Sum([]byte(strings.Join(parts, "&")))
	return hex.EncodeToString(sum[:])
}

var _ payment.Provider = (*XunhuPay)(nil)
