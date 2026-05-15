//go:build unit

package provider

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
)

func TestXunhuPayCreatePayment(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != xunhuEndpointCreate {
			t.Fatalf("path = %q", r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		values, err := url.ParseQuery(string(body))
		if err != nil {
			t.Fatalf("parse body: %v", err)
		}
		if values.Get("payment") != xunhuPaymentWechat {
			t.Fatalf("payment = %q", values.Get("payment"))
		}
		if values.Get("trade_order_id") != "sub2_xh_001" {
			t.Fatalf("trade_order_id = %q", values.Get("trade_order_id"))
		}
		wantHash := xunhuPaySign(map[string]string{
			"version":        xunhuAPIVersion,
			"appid":          "xh-app",
			"trade_order_id": "sub2_xh_001",
			"total_fee":      "9.90",
			"title":          "Balance Recharge",
			"time":           values.Get("time"),
			"notify_url":     "https://merchant.example.com/api/v1/payment/webhook/xunhupay",
			"return_url":     "https://merchant.example.com/payment/result",
			"callback_url":   "https://merchant.example.com/payment/result",
			"payment":        xunhuPaymentWechat,
			"plugins":        "19",
		}, "xh-secret")
		if values.Get("hash") != wantHash {
			t.Fatalf("hash = %q, want %q", values.Get("hash"), wantHash)
		}
		_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"success","url_qrcode":"weixin://qr/test","url":"https://pay.xunhupay.com/redirect"}`))
	}))
	defer server.Close()

	prov, err := NewXunhuPay("19", map[string]string{
		"appId":     "xh-app",
		"appSecret": "xh-secret",
		"apiBase":   server.URL,
		"notifyUrl": "https://merchant.example.com/api/v1/payment/webhook/xunhupay",
		"returnUrl": "https://merchant.example.com/payment/result",
	})
	if err != nil {
		t.Fatalf("NewXunhuPay error: %v", err)
	}

	resp, err := prov.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID:     "sub2_xh_001",
		Amount:      "9.90",
		PaymentType: payment.TypeWxpay,
		Subject:     "Balance Recharge",
	})
	if err != nil {
		t.Fatalf("CreatePayment error: %v", err)
	}
	if resp.PayURL != "https://pay.xunhupay.com/redirect" {
		t.Fatalf("PayURL = %q", resp.PayURL)
	}
	if resp.QRCode != "weixin://qr/test" {
		t.Fatalf("QRCode = %q", resp.QRCode)
	}
}

func TestXunhuPayQueryOrder(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != xunhuEndpointQuery {
			t.Fatalf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"success","data":{"trade_order_id":"sub2_xh_002","open_order_id":"xh_open_002","status":"OD","order_price":"19.80","pay_success_date":"2026-05-15 12:00:00"}}`))
	}))
	defer server.Close()

	prov, err := NewXunhuPay("20", map[string]string{
		"appId":     "xh-app",
		"appSecret": "xh-secret",
		"apiBase":   server.URL,
		"notifyUrl": "https://merchant.example.com/api/v1/payment/webhook/xunhupay",
	})
	if err != nil {
		t.Fatalf("NewXunhuPay error: %v", err)
	}

	resp, err := prov.QueryOrder(context.Background(), "sub2_xh_002")
	if err != nil {
		t.Fatalf("QueryOrder error: %v", err)
	}
	if resp.TradeNo != "xh_open_002" {
		t.Fatalf("TradeNo = %q", resp.TradeNo)
	}
	if resp.Status != payment.ProviderStatusPaid {
		t.Fatalf("Status = %q", resp.Status)
	}
	if resp.Amount != 19.8 {
		t.Fatalf("Amount = %v", resp.Amount)
	}
}

func TestXunhuPayVerifyNotification(t *testing.T) {
	t.Parallel()

	prov, err := NewXunhuPay("21", map[string]string{
		"appId":     "xh-app",
		"appSecret": "xh-secret",
		"apiBase":   "https://api.xunhupay.com",
		"notifyUrl": "https://merchant.example.com/api/v1/payment/webhook/xunhupay",
	})
	if err != nil {
		t.Fatalf("NewXunhuPay error: %v", err)
	}

	params := map[string]string{
		"appid":          "xh-app",
		"trade_order_id": "sub2_xh_003",
		"open_order_id":  "xh_open_003",
		"status":         "OD",
		"order_price":    "29.70",
		"transaction_id": "420000000000003",
		"plugins":        "21",
	}
	params["hash"] = xunhuPaySign(params, "xh-secret")

	values := url.Values{}
	for key, value := range params {
		values.Set(key, value)
	}
	notify, err := prov.VerifyNotification(context.Background(), values.Encode(), nil)
	if err != nil {
		t.Fatalf("VerifyNotification error: %v", err)
	}
	if notify.OrderID != "sub2_xh_003" {
		t.Fatalf("OrderID = %q", notify.OrderID)
	}
	if notify.TradeNo != "xh_open_003" {
		t.Fatalf("TradeNo = %q", notify.TradeNo)
	}
	if notify.Status != payment.ProviderStatusSuccess {
		t.Fatalf("Status = %q", notify.Status)
	}
	if notify.Metadata["transactionId"] != "420000000000003" {
		t.Fatalf("transactionId = %q", notify.Metadata["transactionId"])
	}
}

func TestXunhuPayRefundUsesTradeOrderIDFirst(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != xunhuEndpointRefund {
			t.Fatalf("path = %q", r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		values, err := url.ParseQuery(string(body))
		if err != nil {
			t.Fatalf("parse body: %v", err)
		}
		if got := values.Get("trade_order_id"); got != "sub2_xh_004" {
			t.Fatalf("trade_order_id = %q", got)
		}
		if values.Get("open_order_id") != "" {
			t.Fatalf("open_order_id should be empty, got %q", values.Get("open_order_id"))
		}
		_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"success","data":{"trade_order_id":"sub2_xh_004","open_order_id":"xh_open_004","status":"RD"}}`))
	}))
	defer server.Close()

	prov, err := NewXunhuPay("22", map[string]string{
		"appId":     "xh-app",
		"appSecret": "xh-secret",
		"apiBase":   server.URL,
		"notifyUrl": "https://merchant.example.com/api/v1/payment/webhook/xunhupay",
	})
	if err != nil {
		t.Fatalf("NewXunhuPay error: %v", err)
	}

	resp, err := prov.Refund(context.Background(), payment.RefundRequest{
		OrderID: "sub2_xh_004",
		TradeNo: "xh_open_004",
		Amount:  "9.90",
	})
	if err != nil {
		t.Fatalf("Refund error: %v", err)
	}
	if resp.RefundID != "xh_open_004" {
		t.Fatalf("RefundID = %q", resp.RefundID)
	}
	if resp.Status != payment.ProviderStatusSuccess {
		t.Fatalf("Status = %q", resp.Status)
	}
}

func TestNormalizeXunhuAPIBase(t *testing.T) {
	t.Parallel()

	got := normalizeXunhuAPIBase("https://api.xunhupay.com/payment/do.html")
	if got != "https://api.xunhupay.com" {
		t.Fatalf("normalizeXunhuAPIBase = %q", got)
	}
}

func TestXunhuPaySignIgnoresHashAndEmptyValues(t *testing.T) {
	t.Parallel()

	sign := xunhuPaySign(map[string]string{
		"appid":          "xh-app",
		"trade_order_id": "sub2_test",
		"hash":           "ignored",
		"empty":          "",
	}, "xh-secret")
	if strings.TrimSpace(sign) == "" {
		t.Fatal("expected non-empty sign")
	}
}
