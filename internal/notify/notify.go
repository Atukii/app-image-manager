// Package notify shows desktop notifications over D-Bus (NOTIF-1).
package notify

import (
	"context"
	"fmt"
	"io"

	"github.com/godbus/dbus/v5"

	"appimage-manager/internal/core"
)

// Notifier sends core Notifications to the desktop's notification service,
// writing them to a fallback instead when there is none.
type Notifier struct {
	// Wants filters notifications by kind (config [notify]).
	Wants    func(kind string) bool
	Fallback io.Writer
}

// Notify shows n; it never fails the operation that sent it.
func (nt Notifier) Notify(ctx context.Context, n core.Notification) {
	if nt.Wants != nil && !nt.Wants(n.Kind) {
		return
	}
	if err := send(ctx, n); err != nil && nt.Fallback != nil {
		fmt.Fprintf(nt.Fallback, "aim: %s\n", n.Summary)
	}
}

func send(ctx context.Context, n core.Notification) error {
	conn, err := dbus.ConnectSessionBus(dbus.WithContext(ctx))
	if err != nil {
		return err
	}
	defer conn.Close()
	icon := "system-software-update"
	if n.Kind == "failed" || n.Kind == "digest-mismatch" {
		icon = "dialog-warning"
	}
	return conn.Object("org.freedesktop.Notifications", "/org/freedesktop/Notifications").CallWithContext(ctx,
		"org.freedesktop.Notifications.Notify", 0,
		"AiM", uint32(0), icon, n.Summary, "", []string{}, map[string]dbus.Variant{}, int32(-1),
	).Err
}
