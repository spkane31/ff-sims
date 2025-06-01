package handlers

import "github.com/gin-gonic/gin"

type GetTransactionsResponse struct {
	Transactions []Transaction `json:"transactions"`
}

type TransactionType string

const (
	TransactionTypeDraft  TransactionType = "draft"
	TransactionTypeTrade  TransactionType = "trade"
	TransactionTypeWaiver TransactionType = "waiver"
)

type Transaction struct {
	ID          string              `json:"id"`
	Date        string              `json:"date"`
	Type        TransactionType     `json:"type"`
	Description string              `json:"description"`
	Teams       []string            `json:"teams"`
	Players     []PlayerTransaction `json:"players"`
}

type PlayerTransaction struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Position string  `json:"position"`
	Team     string  `json:"team"`
	Points   float64 `json:"points,omitempty"`
}

func GetTransactions(c *gin.Context) {
	c.JSON(200, GetTransactionsResponse{
		Transactions: []Transaction{
			{
				ID:          "1",
				Date:        "Aug 25, 2024",
				Type:        TransactionTypeDraft,
				Description: "Team A drafted Cooper Kupp with the 8th overall pick.",
				Teams:       []string{"Team A"},
				Players: []PlayerTransaction{
					{
						ID:       "1",
						Name:     "Cooper Kupp",
						Position: "WR",
						Team:     "Los Angeles Rams",
						Points:   256.5,
					},
				},
			},
		},
	})
}
