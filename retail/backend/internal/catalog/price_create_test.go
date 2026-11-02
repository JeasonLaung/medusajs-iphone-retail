package catalog

import (
	"encoding/json"
	"github.com/google/uuid"
	"strings"
	"testing"
)

func TestPriceCreateWire(t *testing.T) {
	in := PriceCreateInput{OperationID: uuid.NewString(), ExpectedRevision: strings.Repeat("a", 64), CurrencyCode: "eur", Amount: "0.000000000000000000"}
	b, _ := json.Marshal(in)
	if _, e := decodePriceCreate(strings.NewReader(string(b))); e != nil {
		t.Fatal(e)
	}
	for _, body := range []string{string(b) + `{}`, strings.Replace(string(b), `"currency_code":"eur"`, `"currency_code":null`, 1), strings.Replace(string(b), `"currency_code":"eur"`, `"currency_code":"EUR"`, 1), strings.Replace(string(b), `"currency_code":"eur"`, `"currency_code":"eur","currency_code":"usd"`, 1), strings.Replace(string(b), `"amount":"0.000000000000000000"`, `"amount":0`, 1), strings.Replace(string(b), `"currency_code":"eur"`, `"currency_code":"eur","price_set_id":"forged"`, 1)} {
		if _, e := decodePriceCreate(strings.NewReader(body)); e == nil {
			t.Fatal("invalid create wire accepted")
		}
	}
}

func TestRegionPriceWire(t *testing.T) {
	in := PriceCreateInput{OperationID: uuid.NewString(), ExpectedRevision: strings.Repeat("a", 64), CurrencyCode: "eur", Amount: "12.123456789012345678", RegionID: "region-1"}
	raw, _ := json.Marshal(in)
	if got, e := decodePriceCreate(strings.NewReader(string(raw))); e != nil || got != in {
		t.Fatal("regional command", e)
	}
	for _, replacement := range []string{`"region_id":null`, `"region_id":""`, `"region_id":1`, `"region_id":"x","region_id":"y"`} {
		bad := strings.Replace(string(raw), `"region_id":"region-1"`, replacement, 1)
		if _, e := decodePriceCreate(strings.NewReader(bad)); e == nil {
			t.Fatal("ambiguous region accepted")
		}
	}
}

func TestQuantityPriceWire(t *testing.T) {
	in := PriceCreateInput{OperationID: uuid.NewString(), ExpectedRevision: strings.Repeat("a", 64), CurrencyCode: "eur", Amount: "80.123456789012345678", MinQuantity: "5", MaxQuantity: "10"}
	for _, bounds := range [][2]string{{"5", "10"}, {"5", ""}, {"", "10"}, {"1", "1"}, {"18446744073709551615", "99999999999999999999"}} {
		in.MinQuantity, in.MaxQuantity = bounds[0], bounds[1]
		b, _ := json.Marshal(in)
		got, e := decodePriceCreate(strings.NewReader(string(b)))
		if e != nil || got != in {
			t.Fatal("quantity wire", e)
		}
	}
	for _, bounds := range [][2]string{{"0", "10"}, {"01", "10"}, {"5.0", "10"}, {"1e1", "20"}, {"-1", "10"}, {"11", "10"}, {"100000000000000000000", ""}} {
		in.MinQuantity, in.MaxQuantity = bounds[0], bounds[1]
		b, _ := json.Marshal(in)
		if _, e := decodePriceCreate(strings.NewReader(string(b))); e == nil {
			t.Fatal("invalid tier accepted", bounds)
		}
	}
	in.MinQuantity, in.MaxQuantity = "5", "10"
	b, _ := json.Marshal(in)
	for _, replacement := range []string{`"min_quantity":null`, `"min_quantity":5`, `"min_quantity":""`, `"min_quantity":"5","min_quantity":"6"`} {
		bad := strings.Replace(string(b), `"min_quantity":"5"`, replacement, 1)
		if _, e := decodePriceCreate(strings.NewReader(bad)); e == nil {
			t.Fatal("ambiguous bound", bad)
		}
	}
}

func TestPriceListMemberWire(t *testing.T) {
	in := PriceCreateInput{OperationID: uuid.NewString(), ExpectedRevision: strings.Repeat("a", 64), CurrencyCode: "eur", Amount: "37.123456789012345678", PriceListID: "list-1", RegionID: "region-1", MinQuantity: "2", MaxQuantity: "3"}
	raw, _ := json.Marshal(in)
	if got, e := decodePriceCreate(strings.NewReader(string(raw))); e != nil || got != in {
		t.Fatal("list member wire", e)
	}
	for _, v := range []string{`"price_list_id":null`, `"price_list_id":1`, `"price_list_id":""`, `"price_list_id":"bad\nvalue"`, `"price_list_id":"list-1","price_list_id":"list-2"`} {
		if _, e := decodePriceCreate(strings.NewReader(strings.Replace(string(raw), `"price_list_id":"list-1"`, v, 1))); e == nil {
			t.Fatal("invalid list member accepted", v)
		}
	}
}
