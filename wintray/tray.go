package wintray

type Item struct {
	ID    uint32
	Title string
}

type Config struct {
	Tooltip   string
	Items     []Item
	OnCommand func(id uint32)
}

func Run(cfg Config) error {
	return run(cfg)
}

func Quit() {
	quit()
}
