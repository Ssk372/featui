package appid

import "testing"

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int
		wantErr bool
	}{
		{name: "número", input: "49520", want: 49520},
		{name: "con espacios", input: "  49520  ", want: 49520},
		{name: "url de tienda", input: "https://store.steampowered.com/app/49520/Borderlands_2/", want: 49520},
		{name: "url de tienda sin nombre", input: "https://store.steampowered.com/app/49520", want: 49520},
		{name: "url de comunidad", input: "https://steamcommunity.com/app/49520", want: 49520},
		{name: "steam run", input: "steam://run/49520", want: 49520},
		{name: "steam store", input: "steam://store/49520", want: 49520},
		{name: "vacío", input: "", wantErr: true},
		{name: "texto", input: "borderlands", wantErr: true},
		{name: "cero", input: "0", wantErr: true},
		{name: "negativo", input: "-5", wantErr: true},
		{name: "url ajena", input: "https://example.com/app/1", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Parse(%q) esperaba error, devolvió %d", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse(%q) error inesperado: %v", tt.input, err)
			}
			if got != tt.want {
				t.Fatalf("Parse(%q) = %d, quiero %d", tt.input, got, tt.want)
			}
		})
	}
}
