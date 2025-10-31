package common

// D-Bus configuration shared between active-window and ignoreApplication
const (
	DbusDestination = "org.gnome.Shell"
	DbusObjectPath  = "/org/gnome/shell/extensions/FocusedWindow"
	DbusInterface   = "org.gnome.shell.extensions.FocusedWindow"
	DbusMethod      = DbusInterface + ".Get"
	
	// Mutter idle monitor D-Bus configuration
	IdleMonitorDestination = "org.gnome.Mutter.IdleMonitor"
	IdleMonitorObjectPath  = "/org/gnome/Mutter/IdleMonitor/Core"
	IdleMonitorInterface   = "org.gnome.Mutter.IdleMonitor"
	IdleMonitorMethod      = IdleMonitorInterface + ".GetIdletime"
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
