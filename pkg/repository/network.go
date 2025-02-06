package repository

import (
	"context"
	"fmt"
)

type networkRepository struct {
	r *Repository
}

func (r *networkRepository) Get(ctx context.Context, id string) (string, error) {
	ip, err := r.r.IP("project1").Get(ctx, "")
	if err != nil {
		return "", err
	}
	fmt.Print(ip)
	return "network", nil
}
