package main

// D-Bus configuration shared between active-window and ignoreApplication
const (
	dbusDestination = "org.gnome.Shell"
	dbusObjectPath  = "/org/gnome/shell/extensions/FocusedWindow"
	dbusInterface   = "org.gnome.shell.extensions.FocusedWindow"
	dbusMethod      = dbusInterface + ".Get"
	
	// Mutter idle monitor D-Bus configuration
	idleMonitorDestination = "org.gnome.Mutter.IdleMonitor"
	idleMonitorObjectPath  = "/org/gnome/Mutter/IdleMonitor/Core"
	idleMonitorInterface   = "org.gnome.Mutter.IdleMonitor"
	idleMonitorMethod      = idleMonitorInterface + ".GetIdletime"
)

// MutterWindow represents the window information from GNOME Shell's FocusedWindow extension
type MutterWindow struct {
	Title              string      `json:"title"`
	WmClass            string      `json:"wm_class"`
	WmClassInstance    string      `json:"wm_class_instance"`
	Pid                int32       `json:"pid"`
	Id                 uint64      `json:"id"`
	Width              int32       `json:"width"`
	Height             int32       `json:"height"`
	X                  int32       `json:"x"`
	Y                  int32       `json:"y"`
	Focus              bool        `json:"focus"`
	InCurrentWorkspace bool        `json:"in_current_workspace"`
	Moveable           bool        `json:"moveable"`
	Resizeable         bool        `json:"resizeable"`
	CanClose           bool        `json:"canclose"`
	CanMaximize        bool        `json:"canmaximize"`
	Maximized          bool        `json:"maximized"`
	CanMinimize        bool        `json:"canminimize"`
	Display            interface{} `json:"display"`
	FrameType          int32       `json:"frame_type"`
	WindowType         int32       `json:"window_type"`
	Layer              int32       `json:"layer"`
	Monitor            int32       `json:"monitor"`
	Role               string      `json:"role"`
	Area               interface{} `json:"area"`
	AreaAll            interface{} `json:"area_all"`
	AreaCust           interface{} `json:"area_cust"`
}
