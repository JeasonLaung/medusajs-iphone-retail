package maildelivery

import (
	"encoding/json"
	"html"
	"net/mail"
	"regexp"
	"strings"
)

type OrderPlaced struct{ To, Reference, Amount, Currency, Locale string }

var orderReference = regexp.MustCompile(`^[A-Za-z0-9_-]{1,191}$`)
var orderAmount = regexp.MustCompile(`^(0|[1-9][0-9]{0,191})(\.[0-9]{1,128})?$`)
var orderCurrency = regexp.MustCompile(`^[a-z]{3}$`)

// The body is frozen before first dispatch. No credential-bearing cart/order
// URL is sent: possession of an email does not grant historical order access.
func (s *Resend) PrepareOrder(in OrderPlaced) ([]byte, error) {
	a, err := mail.ParseAddress(in.To)
	if s == nil || err != nil || a.Address != in.To || a.Name != "" || len(in.To) > 254 || strings.ContainsAny(in.To, "\r\n\x00") || !orderReference.MatchString(in.Reference) || strings.ContainsAny(in.Reference, "\r\n") || !orderAmount.MatchString(in.Amount) || strings.ContainsAny(in.Amount, "\r\n") || !orderCurrency.MatchString(in.Currency) || strings.ContainsAny(in.Currency, "\r\n") || in.Locale != "en" && in.Locale != "zh" {
		return nil, ErrConfig
	}
	title, intro, reference, total := "Order confirmation", "Thank you for your order. We have received your payment and are preparing your order.", "Order", "Amount paid"
	if in.Locale == "zh" {
		title, intro, reference, total = "订单确认", "感谢您的订购。我们已收到付款，正在处理您的订单。", "订单编号", "实付金额"
	}
	amount := in.Amount + " " + strings.ToUpper(in.Currency)
	text := intro + "\n\n" + reference + ": " + in.Reference + "\n" + total + ": " + amount
	body := "<h1>" + title + "</h1><p>" + intro + "</p><p>" + reference + ": " + html.EscapeString(in.Reference) + "</p><p>" + total + ": " + html.EscapeString(amount) + "</p>"
	return json.Marshal(request{From: s.from, To: []string{in.To}, Subject: title + " — " + in.Reference, Text: text, HTML: body, ReplyTo: s.reply})
}
