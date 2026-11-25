package payment

import (
	"fmt"
	"strings"
)

// BRCode monta um Pix copia e cola no padrão EMV do Banco Central: uma
// sequência de campos ID (2 dígitos) + tamanho (2 dígitos) + valor,
// terminada por um CRC16 do conteúdo inteiro.
type BRCode struct {
	Key          string // chave Pix do recebedor
	Description  string
	MerchantName string // até 25 caracteres
	MerchantCity string // até 15 caracteres
	AmountCents  int64
	TxID         string // até 25 caracteres alfanuméricos
}

func (b BRCode) String() string {
	merchant := field("00", "br.gov.bcb.pix") + field("01", b.Key)
	if b.Description != "" {
		merchant += field("02", truncate(b.Description, 40))
	}

	var s strings.Builder
	s.WriteString(field("00", "01"))     // versão do payload
	s.WriteString(field("01", "12"))     // QR de uso único
	s.WriteString(field("26", merchant)) // conta do recebedor (Pix)
	s.WriteString(field("52", "0000"))   // categoria do estabelecimento
	s.WriteString(field("53", "986"))    // moeda: real
	s.WriteString(field("54", fmt.Sprintf("%d.%02d", b.AmountCents/100, b.AmountCents%100)))
	s.WriteString(field("58", "BR"))
	s.WriteString(field("59", truncate(b.MerchantName, 25)))
	s.WriteString(field("60", truncate(b.MerchantCity, 15)))
	s.WriteString(field("62", field("05", truncate(b.TxID, 25))))
	// O campo do CRC entra no próprio cálculo: ID e tamanho, sem o valor.
	s.WriteString("6304")
	return s.String() + fmt.Sprintf("%04X", crc16(s.String()))
}

func field(id, value string) string {
	return fmt.Sprintf("%s%02d%s", id, len(value), value)
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// crc16 é o CRC-16/CCITT-FALSE (polinômio 0x1021, valor inicial 0xFFFF),
// o exigido pelo padrão do Pix.
func crc16(s string) uint16 {
	crc := uint16(0xFFFF)
	for i := 0; i < len(s); i++ {
		crc ^= uint16(s[i]) << 8
		for range 8 {
			if crc&0x8000 != 0 {
				crc = crc<<1 ^ 0x1021
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}
