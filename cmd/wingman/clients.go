package main

import (
	"context"
	"fmt"
	"net/url"

	"github.com/urfave/cli/v3"

	"github.com/chaserensberger/wingman/api"
)

func clientsCommand() *cli.Command {
	return &cli.Command{Name: "clients", Usage: "Manage API clients", Commands: []*cli.Command{
		{Name: "create", Usage: "Create a client and access token", Flags: []cli.Flag{
			&cli.StringFlag{Name: "id", Usage: "Stable client ID, such as cli_reference", Required: true},
			&cli.StringFlag{Name: "name", Usage: "Client display name", Required: true},
		}, Action: runClientCreate},
		{Name: "rotate", Usage: "Rotate a client access token", ArgsUsage: "<client-id>", Action: runClientRotate},
	}}
}

func runClientCreate(ctx context.Context, cmd *cli.Command) error {
	client, err := discoverManagedDaemon(ctx)
	if err != nil {
		return err
	}
	req := api.CreateClientRequest{ID: cmd.String("id"), Name: cmd.String("name")}
	var created api.CreateClientResponse
	if err := client.DoJSON(ctx, "POST", "/clients", req, &created); err != nil {
		return err
	}
	fmt.Fprintf(commandWriter(cmd), "Created client %s (%s)\n\nAccess token: %s\n", created.Client.ID, created.Client.Name, created.Token)
	return nil
}

func runClientRotate(ctx context.Context, cmd *cli.Command) error {
	if cmd.Args().Len() != 1 {
		return fmt.Errorf("expected exactly one client ID")
	}
	client, err := discoverManagedDaemon(ctx)
	if err != nil {
		return err
	}
	var rotated api.CreateClientResponse
	if err := client.DoJSON(ctx, "POST", "/clients/"+url.PathEscape(cmd.Args().First())+"/token", nil, &rotated); err != nil {
		return err
	}
	fmt.Fprintf(commandWriter(cmd), "Rotated access token for %s (%s)\n\nAccess token: %s\n", rotated.Client.ID, rotated.Client.Name, rotated.Token)
	return nil
}
