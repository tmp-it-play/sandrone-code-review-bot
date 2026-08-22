package outbound

type Masker interface {
	Mask(text string) string
}
