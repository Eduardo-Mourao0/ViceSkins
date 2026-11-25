package payment

import (
	"context"
	"crypto/rand"
	"encoding/hex"
)

// demoPixKey é uma chave Pix que não pode existir: o domínio .invalid é
// reservado e nunca resolve. Se alguém ler o QR no app do banco, o banco
// responde "chave não encontrada" e nenhum dinheiro sai.
const demoPixKey = "pagamento@viceskins.invalid"

// Simulated é um provedor Pix sem estado. O "pagamento" é o usuário clicando
// em simular (SimulatePayment), e o service confirma o depósito na mesma
// requisição. Por isso o provedor não precisa lembrar de nada: o banco é a
// fonte da verdade.
//
// Sem estado de propósito: em serverless (Vercel), a requisição que cria o
// Pix e a que simula o pagamento podem cair em instâncias diferentes, e um
// mapa em memória numa não existiria na outra.
type Simulated struct{}

func NewSimulated() *Simulated { return &Simulated{} }

func (s *Simulated) Name() string { return "simulated" }

func (s *Simulated) CreateCharge(_ context.Context, req ChargeRequest) (Charge, error) {
	// ID aleatório, não um contador: um contador recomeçaria do 1 a cada
	// processo e colidiria com cobranças antigas no banco.
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return Charge{}, err
	}
	code := BRCode{
		Key:          demoPixKey,
		Description:  req.Description,
		MerchantName: "VICESKINS DEMO",
		MerchantCity: "SAO PAULO",
		AmountCents:  req.AmountCents,
		TxID:         req.Reference,
	}
	return Charge{ID: "sim_pix_" + hex.EncodeToString(b), BRCode: code.String()}, nil
}

// ChargeStatus: ninguém paga um Pix simulado fora do SimulatePayment, que já
// confirma o depósito na hora. Então, para o job de expiração, um Pix
// pendente que venceu nunca foi pago.
func (s *Simulated) ChargeStatus(context.Context, string) (Status, error) {
	return StatusUnknown, nil
}

// SimulatePayment faz o papel do usuário pagando o Pix no app do banco. O
// service confere antes, no banco, se o depósito é dele e se não venceu.
func (s *Simulated) SimulatePayment(context.Context, string) error {
	return nil
}
