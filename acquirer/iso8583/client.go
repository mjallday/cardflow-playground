package iso8583

import (
	"fmt"
	"time"

	"github.com/alovak/cardflow-playground/acquirer"
	"github.com/moov-io/iso8583"
	iso8583Connection "github.com/moov-io/iso8583-connection"
	"github.com/moov-io/iso8583/field"
	"golang.org/x/exp/slog"
)

type Client struct {
	iso8583Connection *iso8583Connection.Connection
	logger            *slog.Logger
	stanGenerator     STANGenerator
}

type STANGenerator interface {
	Next() string
}

func NewClient(logger *slog.Logger, iso8583ServerAddr string, stanGenerator STANGenerator) (*Client, error) {
	logger = logger.With(slog.String("type", "iso8583-client"), slog.String("addr", iso8583ServerAddr))

	conn, err := iso8583Connection.New(
		iso8583ServerAddr,
		spec,
		readMessageLength,
		writeMessageLength,
		iso8583Connection.SendTimeout(5*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("creating iso8583 connection: %w", err)
	}

	return &Client{
		iso8583Connection: conn,
		logger:            logger,
		stanGenerator:     stanGenerator,
	}, nil
}

func (c *Client) Connect() error {
	c.logger.Info("connecting to ISO 8583 server...")

	if err := c.iso8583Connection.Connect(); err != nil {
		return fmt.Errorf("connecting to ISO 8583 server: %w", err)
	}

	c.logger.Info("connected to ISO 8583 server")
	return nil
}

func (c *Client) AuthorizePayment(payment *acquirer.Payment, card acquirer.Card) (acquirer.AuthorizationResponse, error) {
	c.logger.Info("authorizing payment", slog.String("payment_id", payment.ID))

	requestMessage := iso8583.NewMessage(spec)
	requestData := &AuthorizationRequest{
		MTI:                   field.NewStringValue("0100"),
		PrimaryAccountNumber:  field.NewStringValue(card.Number),
		Amount:                field.NewStringValue(fmt.Sprintf("%d", payment.Amount)),
		Currency:              field.NewStringValue(payment.Currency),
		TransmissionDateTime:  field.NewStringValue(payment.CreatedAt.UTC().Format(time.RFC3339)),
		STAN:                  field.NewStringValue(c.stanGenerator.Next()),
		CardVerificationValue: field.NewStringValue(card.CardVerificationValue),
		ExpirationDate:        field.NewStringValue(card.ExpirationDate),
	}

	err := requestMessage.Marshal(requestData)
	if err != nil {
		return acquirer.AuthorizationResponse{}, fmt.Errorf("marshaling request data: %w", err)
	}

	responseMessage, err := c.iso8583Connection.Send(requestMessage)
	if err != nil {
		return acquirer.AuthorizationResponse{}, fmt.Errorf("sending ISO 8583 message to server: %w", err)
	}

	responseData := &AuthorizationResponse{}
	err = responseMessage.Unmarshal(responseData)
	if err != nil {
		return acquirer.AuthorizationResponse{}, fmt.Errorf("unmarshaling response data: %w", err)
	}

	return acquirer.AuthorizationResponse{
		ApprovalCode:      responseData.ApprovalCode.Value(),
		AuthorizationCode: responseData.AuthorizationCode.Value(),
	}, nil
}
