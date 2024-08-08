package rest

type result struct {
	Url  string `json:"url"`
	Data string `json:"data"`
	Err  string `json:"err"`
}

type node struct {
	Name string `json:"name"`
	Addr string `json:"addr"`
}
