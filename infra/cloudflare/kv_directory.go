package cloudflare

import (
	"context"
	"encoding/json"
	"log"
	"time"
)

const (
	publishInterval = 10 * time.Second
	publishTTL      = 60
	httpTimeout     = 5 * time.Second
)

type Publisher struct {
	AccountID   string
	NamespaceID string
	APIToken    string
	KeyPrefix   string
	ReplicaID   string
	PublicURL   string
	Region      string

	kv *KVClient
}

func New(accountID, namespaceID, apiToken, keyPrefix, replicaID, publicURL, region string) *Publisher {
	if accountID == "" || namespaceID == "" || apiToken == "" || replicaID == "" || publicURL == "" {
		return nil
	}
	if keyPrefix == "" {
		keyPrefix = "replicas:"
	}
	return &Publisher{
		AccountID:   accountID,
		NamespaceID: namespaceID,
		APIToken:    apiToken,
		KeyPrefix:   keyPrefix,
		ReplicaID:   replicaID,
		PublicURL:   publicURL,
		Region:      region,
		kv:          NewKVClient(accountID, namespaceID, apiToken),
	}
}

type entry struct {
	ID        string `json:"id"`
	URL       string `json:"url"`
	Region    string `json:"region,omitempty"`
	UpdatedAt int64  `json:"updated_at"`
}

func (p *Publisher) Run(ctx context.Context) {
	if p == nil {
		return
	}
	log.Printf("[cf-kv] publisher started (replica=%s url=%s prefix=%s)", p.ReplicaID, p.PublicURL, p.KeyPrefix)

	if err := p.publish(ctx); err != nil {
		log.Printf("[cf-kv] initial publish failed: %v", err)
	}

	ticker := time.NewTicker(publishInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			p.delete()
			return
		case <-ticker.C:
			if err := p.publish(ctx); err != nil {
				log.Printf("[cf-kv] publish failed: %v", err)
			}
		}
	}
}

func (p *Publisher) publish(ctx context.Context) error {
	body, err := json.Marshal(entry{
		ID:        p.ReplicaID,
		URL:       p.PublicURL,
		Region:    p.Region,
		UpdatedAt: time.Now().Unix(),
	})
	if err != nil {
		return err
	}
	return p.kv.Put(ctx, p.KeyPrefix+p.ReplicaID, body, publishTTL)
}

func (p *Publisher) delete() {
	ctx, cancel := context.WithTimeout(context.Background(), httpTimeout)
	defer cancel()
	if err := p.kv.Delete(ctx, p.KeyPrefix+p.ReplicaID); err != nil {
		log.Printf("[cf-kv] graceful delete failed: %v", err)
		return
	}
	log.Printf("[cf-kv] entry deleted on shutdown")
}
