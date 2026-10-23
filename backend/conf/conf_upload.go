package conf

type Upload struct {
	Size      int      `yaml:"size"`
	Whitelist []string `yaml:"whitelist"`
	UploadDir string   `yaml:"upload_dir"`
}
