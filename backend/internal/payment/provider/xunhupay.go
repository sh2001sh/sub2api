package provider

import (
	"context"
	"crypto/hmac"
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
	xunhuDefaultAPIBase     = "https://api.xunhupay.com"
	xunhuAPIVersion         = "1.1"
	xunhuHTTPTimeout        = 10 * time.Second
	xunhuMaxResponseSize    = 1 << 20
	xunhuEndpointCreate     = "/payment/do.html"
	xunhuEndpointQuery      = "/payment/query.html"
	xunhuEndpointRefund     = "/payment/refund.html"
	xunhuPaymentWechat      = "wechat"
	xunhuStatusPaid         = "OD"
	xunhuStatusRefunded     = "RD"
	xunhuStatusPending      = "WP"
	xunhuStatusExpired      = "EX"
	xunhuStatusCancelled    = "CD"
	xunhuStatusRefundFailed = "UD"
)

type XunhuPay struct {
	instanceID string
	config     map[string]string
	httpClient *http.Client
}

func NewXunhuPay(instanceID string, config map[string]string) (*XunhuPay, error) {
	for _, k := range []string{"appId", "appSecret", "notifyUrl"} {
		if strings.TrimSpace(config[k]) == "" {
			return nil, fmt.Errorf("xunhupay config missing required key: %s", k)
		}
	}

	cfg := make(map[string]string, len(config))
	for k, v := range config {
		cfg[k] = v
	}
	cfg["apiBase"] = normalizeXunhuAPIBase(cfg["apiBase"])
	if cfg["apiBase"] == "" {
		cfg["apiBase"] = xunhuDefaultAPIBase
	}

	return &XunhuPay{
		instanceID: instanceID,
		config:     cfg,
		httpClient: &http.Client{Timeout: xunhuHTTPTimeout},
	}, nil
}

func (x *XunhuPay) Name() string        { return "XunhuPay" }
func (x *XunhuPay) ProviderKey() string { return payment.TypeXunhuPay }
func (x *XunhuPay) SupportedTypes() []payment.PaymentType {
	return []payment.PaymentType{payment.TypeWxpay}
}

func (x *XunhuPay) MerchantIdentityMetadata() map[string]string {
	if x == nil {
		return nil
	}
	appID := strings.TrimSpace(x.config["appId"])
	if appID == "" {
		return nil
	}
	return map[string]string{"appId": appID}
}

func (x *XunhuPay) CreatePayment(ctx context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	notifyURL := strings.TrimSpace(req.NotifyURL)
	if notifyURL == "" {
		notifyURL = strings.TrimSpace(x.config["notifyUrl"])
	}
	if notifyURL == "" {
		return nil, fmt.Errorf("xunhupay notifyUrl is required")
	}

	returnURL := strings.TrimSpace(req.ReturnURL)
	if returnURL == "" {
		returnURL = strings.TrimSpace(x.config["returnUrl"])
	}

	params := map[string]string{
		"version":        xunhuAPIVersion,
		"appid":          strings.TrimSpace(x.config["appId"]),
		"trade_order_id": strings.TrimSpace(req.OrderID),
		"total_fee":      strings.TrimSpace(req.Amount),
		"title":          strings.TrimSpace(req.Subject),
		"time":           strconv.FormatInt(time.Now().Unix(), 10),
		"notify_url":     notifyURL,
		"payment":        xunhuPaymentWechat,
		"plugins":        x.instanceID,
	}
	if returnURL != "" {
		params["return_url"] = returnURL
		params["callback_url"] = returnURL
	}
	params["hash"] = xunhuPaySign(params, x.config["appSecret"])

	body, err := x.postForm(ctx, x.apiBase()+xunhuEndpointCreate, params)
	if err != nil {
		return nil, fmt.Errorf("xunhupay create: %w", err)
	}

	var resp struct {
		Errcode   int    `json:"errcode"`
		Errmsg    string `json:"errmsg"`
		URLQrcode string `json:"url_qrcode"`
		URL       string `json:"url"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("xunhupay parse create: %w", err)
	}
	if resp.Errcode != 0 {
		return nil, fmt.Errorf("xunhupay error: %s", strings.TrimSpace(resp.Errmsg))
	}

	payURL := strings.TrimSpace(resp.URL)
	qrCode := strings.TrimSpace(resp.URLQrcode)
	if payURL == "" {
		payURL = qrCode
	}
	if qrCode == "" {
		qrCode = payURL
	}

	return &payment.CreatePaymentResponse{
		PayURL: payURL,
		QRCode: qrCode,
	}, nil
}

func (x *XunhuPay) QueryOrder(ctx context.Context, tradeNo string) (*payment.QueryOrderResponse, error) {
	params := map[string]string{
		"appid":          strings.TrimSpace(x.config["appId"]),
		"trade_order_id": strings.TrimSpace(tradeNo),
		"time":           strconv.FormatInt(time.Now().Unix(), 10),
	}
	params["hash"] = xunhuPaySign(params, x.config["appSecret"])

	body, err := x.postForm(ctx, x.apiBase()+xunhuEndpointQuery, params)
	if err != nil {
		return nil, fmt.Errorf("xunhupay query: %w", err)
	}

	var resp struct {
		Errcode int             `json:"errcode"`
		Errmsg  string          `json:"errmsg"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("xunhupay parse query: %w", err)
	}
	if resp.Errcode != 0 {
		return nil, fmt.Errorf("xunhupay query error: %s", strings.TrimSpace(resp.Errmsg))
	}

	var data struct {
		Status         string `json:"status"`
		OrderPrice     string `json:"order_price"`
		OpenOrderID    string `json:"open_order_id"`
		PaySuccessDate string `json:"pay_success_date"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return nil, fmt.Errorf("xunhupay parse query data: %w", err)
	}

	amount, _ := strconv.ParseFloat(strings.TrimSpace(data.OrderPrice), 64)
	return &payment.QueryOrderResponse{
		TradeNo:  firstNonEmpty(strings.TrimSpace(data.OpenOrderID), strings.TrimSpace(tradeNo)),
		Status:   xunhuPayProviderStatus(data.Status),
		Amount:   amount,
		PaidAt:   strings.TrimSpace(data.PaySuccessDate),
		Metadata: x.MerchantIdentityMetadata(),
	}, nil
}

func (x *XunhuPay) VerifyNotification(_ context.Context, rawBody string, _ map[string]string) (*payment.PaymentNotification, error) {
	values, err := url.ParseQuery(rawBody)
	if err != nil {
		return nil, fmt.Errorf("parse notify: %w", err)
	}

	params := make(map[string]string, len(values))
	for key := range values {
		params[key] = values.Get(key)
	}

	hash := strings.TrimSpace(params["hash"])
	if hash == "" {
		return nil, fmt.Errorf("missing hash")
	}
	if !xunhuPayVerifySign(params, x.config["appSecret"], hash) {
		return nil, fmt.Errorf("invalid signature")
	}

	amount, _ := strconv.ParseFloat(strings.TrimSpace(params["order_price"]), 64)
	metadata := x.MerchantIdentityMetadata()
	if appID := strings.TrimSpace(params["appid"]); appID != "" {
		if metadata == nil {
			metadata = map[string]string{}
		}
		metadata["appId"] = appID
	}
	if transactionID := strings.TrimSpace(params["transaction_id"]); transactionID != "" {
		if metadata == nil {
			metadata = map[string]string{}
		}
		metadata["transactionId"] = transactionID
	}

	return &payment.PaymentNotification{
		TradeNo:  strings.TrimSpace(params["open_order_id"]),
		OrderID:  strings.TrimSpace(params["trade_order_id"]),
		Amount:   amount,
		Status:   xunhuPayNotificationStatus(params["status"]),
		RawData:  rawBody,
		Metadata: metadata,
	}, nil
}

func (x *XunhuPay) Refund(ctx context.Context, req payment.RefundRequest) (*payment.RefundResponse, error) {
	params := map[string]string{
		"appid": strings.TrimSpace(x.config["appId"]),
		"time":  strconv.FormatInt(time.Now().Unix(), 10),
	}
	if orderID := strings.TrimSpace(req.OrderID); orderID != "" {
		params["trade_order_id"] = orderID
	} else if tradeNo := strings.TrimSpace(req.TradeNo); tradeNo != "" {
		params["open_order_id"] = tradeNo
	} else {
		return nil, fmt.Errorf("xunhupay refund missing order identifier")
	}
	params["hash"] = xunhuPaySign(params, x.config["appSecret"])

	body, err := x.postForm(ctx, x.apiBase()+xunhuEndpointRefund, params)
	if err != nil {
		return nil, fmt.Errorf("xunhupay refund: %w", err)
	}

	var resp struct {
		Errcode int             `json:"errcode"`
		Errmsg  string          `json:"errmsg"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("xunhupay parse refund: %w", err)
	}
	if resp.Errcode != 0 {
		return nil, fmt.Errorf("xunhupay refund error: %s", strings.TrimSpace(resp.Errmsg))
	}

	var data struct {
		TradeOrderID string `json:"trade_order_id"`
		OpenOrderID  string `json:"open_order_id"`
		Status       string `json:"status"`
	}
	if len(resp.Data) > 0 && string(resp.Data) != "null" {
		if err := json.Unmarshal(resp.Data, &data); err != nil {
			return nil, fmt.Errorf("xunhupay parse refund data: %w", err)
		}
	}

	return &payment.RefundResponse{
		RefundID: firstNonEmpty(strings.TrimSpace(data.OpenOrderID), strings.TrimSpace(data.TradeOrderID), strings.TrimSpace(req.TradeNo), strings.TrimSpace(req.OrderID)),
		Status:   payment.ProviderStatusSuccess,
	}, nil
}

func (x *XunhuPay) apiBase() string {
	if x == nil {
		return xunhuDefaultAPIBase
	}
	base := normalizeXunhuAPIBase(x.config["apiBase"])
	if base == "" {
		return xunhuDefaultAPIBase
	}
	return base
}

func normalizeXunhuAPIBase(apiBase string) string {
	base := strings.TrimSpace(apiBase)
	if base == "" {
		return ""
	}
	if parsed, err := url.Parse(base); err == nil && parsed.Scheme != "" && parsed.Host != "" {
		parsed.RawQuery = ""
		parsed.Fragment = ""
		parsed.RawPath = ""
		parsed.Path = trimXunhuEndpointPath(parsed.Path)
		return strings.TrimRight(parsed.String(), "/")
	}
	return strings.TrimRight(trimXunhuEndpointPath(base), "/")
}

func trimXunhuEndpointPath(path string) string {
	path = strings.TrimRight(strings.TrimSpace(path), "/")
	lower := strings.ToLower(path)
	for _, endpoint := range []string{xunhuEndpointCreate, xunhuEndpointQuery, xunhuEndpointRefund} {
		if strings.HasSuffix(lower, endpoint) {
			return strings.TrimRight(path[:len(path)-len(endpoint)], "/")
		}
	}
	return path
}

func (x *XunhuPay) postForm(ctx context.Context, endpoint string, params map[string]string) ([]byte, error) {
	form := url.Values{}
	for key, value := range params {
		form.Set(key, value)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := x.httpClient
	if client == nil {
		client = &http.Client{Timeout: xunhuHTTPTimeout}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, xunhuMaxResponseSize))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, nil
}

func xunhuPaySign(params map[string]string, appSecret string) string {
	keys := make([]string, 0, len(params))
	for key, value := range params {
		if key == "hash" || strings.TrimSpace(value) == "" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var buf strings.Builder
	for i, key := range keys {
		if i > 0 {
			_ = buf.WriteByte('&')
		}
		_, _ = buf.WriteString(key + "=" + params[key])
	}
	_, _ = buf.WriteString(appSecret)
	sum := md5.Sum([]byte(buf.String()))
	return hex.EncodeToString(sum[:])
}

func xunhuPayVerifySign(params map[string]string, appSecret string, hash string) bool {
	return hmac.Equal([]byte(xunhuPaySign(params, appSecret)), []byte(strings.TrimSpace(hash)))
}

func xunhuPayProviderStatus(status string) string {
	switch strings.TrimSpace(strings.ToUpper(status)) {
	case xunhuStatusPaid:
		return payment.ProviderStatusPaid
	case xunhuStatusRefunded:
		return payment.ProviderStatusRefunded
	case xunhuStatusPending:
		return payment.ProviderStatusPending
	case xunhuStatusExpired, xunhuStatusCancelled, xunhuStatusRefundFailed:
		return payment.ProviderStatusFailed
	default:
		return payment.ProviderStatusPending
	}
}

func xunhuPayNotificationStatus(status string) string {
	if strings.EqualFold(strings.TrimSpace(status), xunhuStatusPaid) {
		return payment.ProviderStatusSuccess
	}
	return payment.ProviderStatusFailed
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
