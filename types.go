package streamers

type Stream struct {
	Username     string `json:"user_name"`
	UserId       string `json:"user_id"`
	Title        string `json:"title"`
	ThumbnailUrl string `json:"thumbnail_url"`
	Viewers      int    `json:"viewer_count"`
	GameId       string `json:"game_id"`
	Game         Game   `json:"game"`
}

type Game struct {
	Id     string `json:"id"`
	Name   string `json:"name"`
	BoxArt string `json:"box_art_url"`
}

type Streamers struct {
	Total   int
	Streams []Stream `json:"data"`
}
type ExtStreamsOut struct {
	Cursor   string `json:"cursor"`
	Channels []struct {
		Id       string `json:"id"`
		Username string `json:"username"`
		Game     string `json:"game"`
		Title    string `json:"title"`
		Views    string `json:"view_count"`
	} `json:"channels"`
}
