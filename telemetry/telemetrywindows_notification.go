package telemetry

import (
    "syscall"
    "sync"

    "github.com/lxn/win"
)

var notificationMu sync.Mutex
var notificationShowing bool
var notifiedCriticalFaults = make(map[string]bool)

func ShowSCADANotification() {

    notificationMu.Lock()

    if notificationShowing {
        notificationMu.Unlock()
        return
    }

    notificationShowing = true
    notificationMu.Unlock()

    defer func() {
        notificationMu.Lock()
        notificationShowing = false
        notificationMu.Unlock()
    }()

    title := syscall.StringToUTF16Ptr("POWERGUARD SCADA ALERT")

    message := syscall.StringToUTF16Ptr(
        "There is a problem in your SCADA application.\n\n" +
            "Please check the SCADA system immediately.",
    )

    win.MessageBox(
        0,
        message,
        title,
        win.MB_OK|
            win.MB_ICONWARNING|
            win.MB_SYSTEMMODAL|
            win.MB_SETFOREGROUND|
            win.MB_TOPMOST,
    )
}