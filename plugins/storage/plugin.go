// Package storageplugin wires the storage data layer into a session as a Plugin.
//
// The plugin packages two contributions behind a single Install call:
//
//   - A BeforeRun hook that loads the session's existing history from
//     the Store, returning it as the loop's initial message slice.
//   - A Sink that listens for loop.MessageEvent and persists each
//     completed message via Store.AppendMessage as it lands.
//
// Together these give a session full-cycle persistence (load + save)
// without the SDK or loop core ever importing storage. The same code
// powers the HTTP server's session handlers.
//
// # Why a plugin
//
// The existing session.WithMessageSink option is a fine primitive for
// ad-hoc message observation (logging, metrics) and remains supported.
// Persistence is more than observation: it has a load side too, and the
// load and save sides must agree on a session id. Packaging both behind
// a single NewPlugin(store, sessionID) call means consumers can't
// accidentally wire one without the other.
//
// # Usage
//
//	store, _ := storage.NewSQLiteStore("/path/to/wingman.db")
//	sess, _ := store.GetSession(sessionID) // ensure the session exists
//	s := session.New(
//	    session.WithModel(model),
//	    session.WithPlugin(storageplugin.NewPlugin(store, sess.ID)),
//	)
//
// Errors from Store.AppendMessage are logged and swallowed: a single
// sqlite hiccup shouldn't kill an in-flight run. Errors from
// Store.GetSession during BeforeRun do fail the run cleanly, since
// proceeding without a known starting state would silently desynchronize
// the in-memory and on-disk transcripts.
package storageplugin

import (
	"context"
	"fmt"
	"log"

	"github.com/chaserensberger/wingman/storage"
	"github.com/chaserensberger/wingman/wingagent/loop"
	"github.com/chaserensberger/wingman/wingagent/plugin"
	"github.com/chaserensberger/wingman/wingmodels"
)

// PluginName is the stable identifier returned by Plugin.Name. Exposed
// as a constant so observability and uniqueness checks can reference it
// without string-literal drift.
const PluginName = "storage"

// NewPlugin returns a Plugin that loads sessionID's history during
// BeforeRun and persists every loop.MessageEvent via store.AppendMessage.
//
// The Plugin is bound to a single sessionID; reuse across sessions is
// not supported (and would conflate transcripts). Construct a fresh
// Plugin per session activation, which is what the HTTP server does.
func NewPlugin(store storage.Store, sessionID string) plugin.Plugin {
	return &storagePlugin{store: store, sessionID: sessionID}
}

type storagePlugin struct {
	store     storage.Store
	sessionID string
}

// Name implements plugin.Plugin.
func (p *storagePlugin) Name() string { return PluginName }

// Install implements plugin.Plugin. Wires the load (BeforeRun) and save
// (sink) seams to the configured Store and session id.
func (p *storagePlugin) Install(r *plugin.Registry) error {
	if p.store == nil {
		return fmt.Errorf("storage plugin: nil store")
	}
	if p.sessionID == "" {
		return fmt.Errorf("storage plugin: empty sessionID")
	}

	r.RegisterBeforeRun(func(_ context.Context, current []wingmodels.Message) ([]wingmodels.Message, error) {
		sess, err := p.store.GetSession(p.sessionID)
		if err != nil {
			return nil, fmt.Errorf("storage plugin: load session %s: %w", p.sessionID, err)
		}
		// Honor any history a prior BeforeRun contributed. The
		// expected case is current == empty (storage is the first
		// BeforeRun installed), but composing under a header-injection
		// plugin should still work cleanly.
		return append(append([]wingmodels.Message(nil), current...), sess.History...), nil
	})

	r.RegisterSink(loop.SinkFunc(func(e loop.Event) {
		me, ok := e.(loop.MessageEvent)
		if !ok {
			return
		}
		if err := p.store.AppendMessage(p.sessionID, me.Message); err != nil {
			// Log-and-continue: persistence failures shouldn't kill
			// in-flight runs. The transcript on disk may end up with
			// gaps; the in-memory transcript (in the loop) is still
			// correct for the caller's Result.
			log.Printf("wingman storage plugin: append message to %s: %v", p.sessionID, err)
		}
	}))

	return nil
}
