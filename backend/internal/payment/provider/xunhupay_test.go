//go:build unit

package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

func TestNewXunhuPayValidatesConfig(t *testing.T) {
	t.Parallel()

	_, err := NewXunhuPay("1", map[string]string{
		"appId":     "app-test",
		"notifyUrl": "https://merchant.example.com/notify",
		"returnUrl": "https://merchant.example.com/return",
	})
	require.ErrorContains(t, err, "appSecret")

	prov, err := NewXunhuPay("1", map[string]string{
		"appId":     "app-test",
		"appSecret": "secret-test",
		"notifyUrl": "https://merchant.example.com/notify",
		"returnUrl": "https://merchant.example.com/return",
	})
	require.NoError(t, err)
	require.Equal(t, payment.TypeXunhuPay, prov.ProviderKey())
	require.Equal(t, []payment.PaymentType{payment.TypeAlipay, payment.TypeWxpay}, prov.SupportedTypes())
	require.Equal(t, xunhuPayDefaultAPIBase, prov.config["apiBase"])
}

func TestXunhuPayCreatePaymentUsesConfiguredURLs(t *testing.T) {
	t.Parallel()

	var payload map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/payment/do.html", r.URL.Path)
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(body, &payload))
		_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"success","url":"https://pay.example.com/h5","url_qrcode":"weixin://qr/pay","order_id":"xh_order_123"}`))
	}))
	defer server.Close()

	prov := mustTestXunhuPayProvider(t, server)
	resp, err := prov.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID: "sub2_order",
		Amount:  "12.34",
		Subject: "Sub2API 12.34 CNY",
	})
	require.NoError(t, err)
	require.Equal(t, "xh_order_123", resp.TradeNo)
	require.Equal(t, "https://pay.example.com/h5", resp.PayURL)
	require.Equal(t, "weixin://qr/pay", resp.QRCode)
	require.Equal(t, "sub2_order", payload["trade_order_id"])
	require.Equal(t, "12.34", payload["total_fee"])
	require.Equal(t, "https://merchant.example.com/notify", payload["notify_url"])
	require.Equal(t, "https://merchant.example.com/return", payload["return_url"])
	require.NotEmpty(t, payload["hash"])
}

func TestXunhuPayQueryOrderMapsPaidStatus(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/payment/query.html", r.URL.Path)
		_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"success","data":{"total_fee":"12.34","status":"OD","trade_order_id":"sub2_order","transaction_id":"txn_123","appid":"app-test"}}`))
	}))
	defer server.Close()

	prov := mustTestXunhuPayProvider(t, server)
	resp, err := prov.QueryOrder(context.Background(), "sub2_order")
	require.NoError(t, err)
	require.Equal(t, payment.ProviderStatusPaid, resp.Status)
	require.Equal(t, "txn_123", resp.TradeNo)
	require.InDelta(t, 12.34, resp.Amount, 0.0001)
	require.Equal(t, "app-test", resp.Metadata["appid"])
	require.Equal(t, "OD", resp.Metadata["status"])
}

func TestXunhuPayVerifyNotification(t *testing.T) {
	t.Parallel()

	prov, err := NewXunhuPay("1", map[string]string{
		"appId":     "app-test",
		"appSecret": "secret-test",
		"notifyUrl": "https://merchant.example.com/notify",
		"returnUrl": "https://merchant.example.com/return",
	})
	require.NoError(t, err)

	params := map[string]string{
		"appid":          "app-test",
		"trade_order_id": "sub2_order",
		"transaction_id": "txn_123",
		"total_fee":      "12.34",
		"status":         "OD",
	}
	params["hash"] = xunhuPaySign(params, "secret-test")
	rawBody := formEncodedPayload(params)

	notification, err := prov.VerifyNotification(context.Background(), rawBody, nil)
	require.NoError(t, err)
	require.NotNil(t, notification)
	require.Equal(t, "sub2_order", notification.OrderID)
	require.Equal(t, "txn_123", notification.TradeNo)
	require.InDelta(t, 12.34, notification.Amount, 0.0001)
	require.Equal(t, payment.ProviderStatusSuccess, notification.Status)
	require.Equal(t, "app-test", notification.Metadata["appid"])
	require.Equal(t, "OD", notification.Metadata["status"])

	params["hash"] = strings.Repeat("0", 32)
	_, err = prov.VerifyNotification(context.Background(), formEncodedPayload(params), nil)
	require.ErrorContains(t, err, "invalid hash")
}

func TestXunhuPayRefundMapsStatus(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/payment/refund.html", r.URL.Path)
		_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"success","refund_order_id":"refund_123","refund_status":"CD"}`))
	}))
	defer server.Close()

	prov := mustTestXunhuPayProvider(t, server)
	resp, err := prov.Refund(context.Background(), payment.RefundRequest{
		OrderID: "sub2_order",
		Amount:  "12.34",
		Reason:  "test refund",
	})
	require.NoError(t, err)
	require.Equal(t, "refund_123", resp.RefundID)
	require.Equal(t, payment.ProviderStatusSuccess, resp.Status)
}

func mustTestXunhuPayProvider(t *testing.T, server *httptest.Server) *XunhuPay {
	t.Helper()

	prov, err := NewXunhuPay("1", map[string]string{
		"appId":     "app-test",
		"appSecret": "secret-test",
		"apiBase":   server.URL + "/payment",
		"notifyUrl": "https://merchant.example.com/notify",
		"returnUrl": "https://merchant.example.com/return",
	})
	require.NoError(t, err)
	prov.httpClient = server.Client()
	return prov
}

func formEncodedPayload(params map[string]string) string {
	parts := make([]string, 0, len(params))
	for key, value := range params {
		parts = append(parts, key+"="+value)
	}
	return strings.Join(parts, "&")
}
