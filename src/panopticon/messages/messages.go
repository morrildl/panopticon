package messages

type Camera struct {
	Name           string
	ID             string
	AspectRatio    string
	LocalTime      string
	LocalDate      string
	Message        string
	LatestEvent    string
	LatestImageURL string
}

type State struct {
	Cameras     []*Camera
	ServiceName string
}
