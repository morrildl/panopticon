package messages

type Camera struct {
	// core information about the camera
	Name        string
	ID          string
	AspectRatio string

	// current state information
	Sleeping  bool
	Offline   bool
	LocalTime string
	LocalDate string

	// information about recent activity
	Message          string
	LatestTime       string
	LatestDate       string
	LatestHandle     string
	RecentHandles    []string
	PinnedHandles    []string
	TimelapseHandles []string
	MotionHandles    []string
}

type State struct {
	Cameras      []*Camera
	ServiceName  string
	DefaultImage string
}
