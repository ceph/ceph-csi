package kp

import (
	"context"
	"fmt"
	"time"
)

const (
	path = "key_rings"
)

type KeyRing struct {
	ID           string     `json:"id,omitempty"`
	CreationDate *time.Time `json:"creationDate,omitempty"`
	CreatedBy    string     `json:"createdBy,omitempty"`
}

type KeyRings struct {
	Metadata KeysMetadata `json:"metadata"`
	KeyRings    []KeyRing       `json:"resources"`
}

// CreateRing method creates a key ring in the instance with the provided name
// For information please refer to the link below:
// https://cloud.ibm.com/docs/key-protect?topic=key-protect-managing-key-rings#create-key-ring-api
func (c *Client) CreateKeyRing(ctx context.Context, id string) error {

	req, err := c.newRequest("POST", fmt.Sprintf(path+"/%s", id), nil)
	if err != nil {
		return err
	}

	_, err = c.do(ctx, req, nil)
	if err != nil {
		return err
	}

	return nil
}

// GetRings method retrieves all the key rings associated with the instance
// For information please refer to the link below:
// https://cloud.ibm.com/docs/key-protect?topic=key-protect-managing-key-rings#list-key-ring-api
func (c *Client) GetKeyRings(ctx context.Context) (*KeyRings, error) {
	rings := KeyRings{}
	req, err := c.newRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}

	_, err = c.do(ctx, req, &rings)
	if err != nil {
		return nil, err
	}

	return &rings, nil
}

// DeleteRing method deletes the key ring with the provided name in the instance
// For information please refer to the link below:
// https://cloud.ibm.com/docs/key-protect?topic=key-protect-managing-key-rings#delete-key-ring-api
func (c *Client) DeleteKeyRing(ctx context.Context, id string) error {
	req, err := c.newRequest("DELETE", fmt.Sprintf(path+"/%s", id), nil)
	if err != nil {
		return err
	}

	_, err = c.do(ctx, req, nil)
	if err != nil {
		return err
	}

	return nil
}
