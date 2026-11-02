package maildelivery

import (
	"encoding/json"
	"html"
	"net/mail"
	"strings"
)

type CustomerEvent struct{ To, Reference, Kind, Action, Locale string }

// These notices disclose only the resource reference and the committed event.
// Inspection notes, addresses, bank details and bearer links are never included.
func (s *Resend) PrepareEvent(in CustomerEvent) ([]byte, error) {
	a, e := mail.ParseAddress(in.To)
	if s == nil || e != nil || a.Name != "" || a.Address != in.To || len(in.To) > 254 || strings.ContainsAny(in.To, "\r\n\x00") || !orderReference.MatchString(in.Reference) || in.Locale != "en" && in.Locale != "zh" {
		return nil, ErrConfig
	}
	titles := map[string][2]string{
		"payment.received":       {"Payment received", "付款已收到"},
		"refund.created":         {"Refund completed", "退款已完成"},
		"shipment.created":       {"Order shipped", "订单已发货"},
		"delivery.created":       {"Delivery updated", "配送已更新"},
		"order.canceled":         {"Order canceled", "订单已取消"},
		"after_sales.processing": {"After-sales request processing", "售后申请处理中"},
		"after_sales.completed":  {"After-sales request completed", "售后申请已完成"},
		"after_sales.rejected":   {"After-sales request declined", "售后申请未通过"},
	}
	if in.Kind == "buyback.updated" {
		titles = map[string][2]string{
			"review_photo":     {"Please retake your device photo", "请重新拍摄设备照片"},
			"approve_shipping": {"Your device is ready to send", "设备已获准寄出"},
			"mark_received":    {"Your device has been received", "设备已收到"},
			"final_offer":      {"Your final buyback offer is ready", "回收最终报价已就绪"},
			"request_return":   {"Device return requested", "已申请退回设备"},
			"mark_returning":   {"Your device is being returned", "设备正在退回"},
			"mark_returned":    {"Your device return is complete", "设备退回已完成"},
			"mark_paid":        {"Buyback payment recorded", "回收付款已登记"},
			"cancel":           {"Buyback request canceled", "回收申请已取消"},
		}
	} else if in.Action != "" {
		return nil, ErrConfig
	}
	key := in.Kind
	if in.Kind == "buyback.updated" {
		key = in.Action
	}
	pair, ok := titles[key]
	if !ok {
		return nil, ErrConfig
	}
	title, intro, label := pair[0], "Sign in to your EuroRefurb account to view the details.", "Reference"
	if in.Locale == "zh" {
		title, intro, label = pair[1], "请登录您的 EuroRefurb 账号查看详情。", "业务编号"
	}
	text := title + "\n\n" + label + ": " + in.Reference + "\n\n" + intro
	body := "<h1>" + html.EscapeString(title) + "</h1><p>" + label + ": " + html.EscapeString(in.Reference) + "</p><p>" + intro + "</p>"
	return json.Marshal(request{From: s.from, To: []string{in.To}, ReplyTo: s.reply, Subject: title + " — " + in.Reference, Text: text, HTML: body})
}
