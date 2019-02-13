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
	Message      string
	LatestTime   string
	LatestDate   string
	LatestHandle string
	Recent       []*ImageMeta
	Pinned       []*ImageMeta
	Timelapse    []*ImageMeta
	Motion       []*ImageMeta
}

type State struct {
	Cameras      []*Camera
	ServiceName  string
	DefaultImage string
}

type ImageMeta struct {
	Handle   string
	Camera   string `json:",omitempty"`
	Time     string `json:",omitempty"`
	Date     string `json:",omitempty"`
	HasVideo bool
}

type ImageList struct {
	Camera string
	Total  int
	Images []*ImageMeta
}
