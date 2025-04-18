package internal

import (
	"fmt"
	"os"

	"github.com/pinecone-io/go-pinecone/pinecone"
)

type Pinecone struct {
	*pinecone.Client
}

func NewPineconeClient() (*Pinecone, error) {
	pineconeKey := os.Getenv("PINECONE_API_KEY")
	if pineconeKey == "" {
		return nil, fmt.Errorf("PINECONE_API_KEY is not set")
	}

	pc, err := pinecone.NewClient(pinecone.NewClientParams{
		ApiKey: pineconeKey,
	})
	if err != nil {
		return nil, fmt.Errorf("create Client: %w", err)
	}

	return &Pinecone{
		Client: pc,
	}, nil
}
