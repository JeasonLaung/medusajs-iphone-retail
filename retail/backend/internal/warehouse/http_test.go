package warehouse

import (
	"strings"
	"testing"
)

func TestStrictWarehouseCommand(t *testing.T) {
	valid := `{"operation_id":"2ca432d3-cea0-4a4e-9433-cbc0d047432c","expected_revision":"1","id":"","name":"仓库","address":{"address_1":"Street","address_2":"","company":"","city":"","country_code":"dk","phone":"","province":"","postal_code":"0012"}}`
	if in, e := decode(strings.NewReader(valid)); e != nil || in.Address.PostalCode != "0012" {
		t.Fatal("valid command rejected", e)
	}
	for _, bad := range []string{`null`, `{}`, valid + `{}`, strings.Replace(valid, `"name":"仓库"`, `"name":"仓库","name":"Other"`, 1), strings.Replace(valid, `"address_1":"Street"`, `"address_1":"Street","address_1":"Other"`, 1), strings.Replace(valid, `"phone":""`, `"phone":null`, 1), strings.Replace(valid, `"city":""`, `"city":"","unknown":""`, 1), strings.Replace(valid, `"expected_revision":"1"`, `"expected_revision":"01"`, 1), strings.Replace(valid, `"country_code":"dk"`, `"country_code":"DK"`, 1), strings.Replace(valid, `"name":"仓库"`, `"name":"line\nother"`, 1), strings.Replace(valid, "仓库", string([]byte{0xff}), 1)} {
		if _, e := decode(strings.NewReader(bad)); e == nil {
			t.Fatal("invalid command accepted")
		}
	}
}
