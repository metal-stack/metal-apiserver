package ipam

import (
	"context"
	"errors"

	connectv1 "connectrpc.com/connect"
	connectv2 "connectrpc.com/connect/v2"
	ipamv1 "github.com/metal-stack/go-ipam/api/v1"
	ipamv1connect "github.com/metal-stack/go-ipam/api/v1/apiv1connect"
)

// Client wraps an ipamv1connect.IpamServiceClient.
//
// go-ipam is still generated with connect-go v1, so the errors it returns are
// *connectrpc.com/connect.Error values. The metal-stack api errorutil only
// classifies connect-go v2 errors, therefore this wrapper converts any v1
// connect error into its v2 counterpart so errorutil.IsNotFound and friends
// keep working.
type Client struct {
	ipamv1connect.IpamServiceClient
}

// New wraps the given ipam client.
func New(client ipamv1connect.IpamServiceClient) *Client {
	return &Client{IpamServiceClient: client}
}

func convertErr(err error) error {
	if err == nil {
		return nil
	}

	if v1err, ok := errors.AsType[*connectv1.Error](err); ok {
		return connectv2.NewError(connectv2.Code(v1err.Code()), v1err.Message()).WithCause(v1err)
	}

	return err
}

func (c *Client) CreatePrefix(ctx context.Context, req *ipamv1.CreatePrefixRequest) (*ipamv1.CreatePrefixResponse, error) {
	res, err := c.IpamServiceClient.CreatePrefix(ctx, req)
	return res, convertErr(err)
}

func (c *Client) DeletePrefix(ctx context.Context, req *ipamv1.DeletePrefixRequest) (*ipamv1.DeletePrefixResponse, error) {
	res, err := c.IpamServiceClient.DeletePrefix(ctx, req)
	return res, convertErr(err)
}

func (c *Client) GetPrefix(ctx context.Context, req *ipamv1.GetPrefixRequest) (*ipamv1.GetPrefixResponse, error) {
	res, err := c.IpamServiceClient.GetPrefix(ctx, req)
	return res, convertErr(err)
}

func (c *Client) ListPrefixes(ctx context.Context, req *ipamv1.ListPrefixesRequest) (*ipamv1.ListPrefixesResponse, error) {
	res, err := c.IpamServiceClient.ListPrefixes(ctx, req)
	return res, convertErr(err)
}

func (c *Client) PrefixUsage(ctx context.Context, req *ipamv1.PrefixUsageRequest) (*ipamv1.PrefixUsageResponse, error) {
	res, err := c.IpamServiceClient.PrefixUsage(ctx, req)
	return res, convertErr(err)
}

func (c *Client) AcquireChildPrefix(ctx context.Context, req *ipamv1.AcquireChildPrefixRequest) (*ipamv1.AcquireChildPrefixResponse, error) {
	res, err := c.IpamServiceClient.AcquireChildPrefix(ctx, req)
	return res, convertErr(err)
}

func (c *Client) ReleaseChildPrefix(ctx context.Context, req *ipamv1.ReleaseChildPrefixRequest) (*ipamv1.ReleaseChildPrefixResponse, error) {
	res, err := c.IpamServiceClient.ReleaseChildPrefix(ctx, req)
	return res, convertErr(err)
}

func (c *Client) AcquireIP(ctx context.Context, req *ipamv1.AcquireIPRequest) (*ipamv1.AcquireIPResponse, error) {
	res, err := c.IpamServiceClient.AcquireIP(ctx, req)
	return res, convertErr(err)
}

func (c *Client) ReleaseIP(ctx context.Context, req *ipamv1.ReleaseIPRequest) (*ipamv1.ReleaseIPResponse, error) {
	res, err := c.IpamServiceClient.ReleaseIP(ctx, req)
	return res, convertErr(err)
}

func (c *Client) Dump(ctx context.Context, req *ipamv1.DumpRequest) (*ipamv1.DumpResponse, error) {
	res, err := c.IpamServiceClient.Dump(ctx, req)
	return res, convertErr(err)
}

func (c *Client) Load(ctx context.Context, req *ipamv1.LoadRequest) (*ipamv1.LoadResponse, error) {
	res, err := c.IpamServiceClient.Load(ctx, req)
	return res, convertErr(err)
}

func (c *Client) CreateNamespace(ctx context.Context, req *ipamv1.CreateNamespaceRequest) (*ipamv1.CreateNamespaceResponse, error) {
	res, err := c.IpamServiceClient.CreateNamespace(ctx, req)
	return res, convertErr(err)
}

func (c *Client) ListNamespaces(ctx context.Context, req *ipamv1.ListNamespacesRequest) (*ipamv1.ListNamespacesResponse, error) {
	res, err := c.IpamServiceClient.ListNamespaces(ctx, req)
	return res, convertErr(err)
}

func (c *Client) DeleteNamespace(ctx context.Context, req *ipamv1.DeleteNamespaceRequest) (*ipamv1.DeleteNamespaceResponse, error) {
	res, err := c.IpamServiceClient.DeleteNamespace(ctx, req)
	return res, convertErr(err)
}

func (c *Client) Version(ctx context.Context, req *ipamv1.VersionRequest) (*ipamv1.VersionResponse, error) {
	res, err := c.IpamServiceClient.Version(ctx, req)
	return res, convertErr(err)
}
