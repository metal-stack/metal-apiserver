package repository

import (
	"context"
	"fmt"
)

type ipRepository struct {
	r *Repository
}

type ipUnscopedRepository struct {
	r *Repository
}

func (r *ipRepository) Get(ctx context.Context, id string) (string, error) {

	ip, err := r.r.ds.IP().Get(ctx, id)
	if err != nil {
		return "", err
	}

	fmt.Print(ip)

	nw, _ := r.r.Network().Get(ctx, "asdf")
	fmt.Print(nw)
	return "1.2.3.4", nil
}

func (ur *ipUnscopedRepository) List() []string {
	return []string{"1.2.3.4", "fe80::1"}
}
