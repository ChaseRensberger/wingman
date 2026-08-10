package main

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/chaserensberger/wingman/api"
)

func clientsCommand() *cli.Command {
	return &cli.Command{Name: "clients", Usage: "Manage API clients", Commands: []*cli.Command{
		{Name: "create", Usage: "Register a client", Flags: []cli.Flag{
			&cli.StringFlag{Name: "id", Usage: "Stable client ID, such as cli_reference", Required: true},
			&cli.StringFlag{Name: "name", Usage: "Client display name", Required: true},
		}, Action: runClientCreate},
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
	fmt.Fprintf(commandWriter(cmd), "Registered client %s (%s)\n", created.Client.ID, created.Client.Name)
	return nil
}
