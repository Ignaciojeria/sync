package design

// Document representa un DESIGN.md parseado casi tal como viene del archivo.
type Document struct {
	Version     string
	Name        string
	Description string
	Colors      map[string]string
	Typography  map[string]TypographyToken
	Rounded     map[string]string
	Spacing     map[string]string
	Components  map[string]map[string]string
	XPi         XPiExtension
	Body        string
}

// TypographyToken representa un nivel tipográfico del spec DESIGN.md.
type TypographyToken struct {
	FontFamily    string
	FontSize      string
	FontWeight    string
	LineHeight    string
	LetterSpacing string
	FontFeature   string
	FontVariation string
}

// XPiExtension agrupa extensiones runtime del proyecto.
type XPiExtension struct {
	ThemeID     string
	ColorScheme string
	DaisyUI     map[string]string
}

// ResolvedTheme representa el tema normalizado y listo para compilar.
type ResolvedTheme struct {
	ID          string
	Name        string
	Description string
	ColorScheme string
	Colors      map[string]string
	Typography  map[string]TypographyToken
	Rounded     map[string]string
	Spacing     map[string]string
	Components  map[string]map[string]string
	DaisyUI     map[string]string
	Warnings    []string
}

// ThemeCSS representa la compilación runtime del tema a CSS.
type ThemeCSS struct {
	ThemeID string
	Content string
	ETag    string
}
