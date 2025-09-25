package schema

type MarkdownSettings struct {
	Document              MarkdownStyle `yaml:"document,omitempty" json:"document,omitempty" mapstructure:"document"`
	BlockQuote            MarkdownStyle `yaml:"block_quote,omitempty" json:"block_quote,omitempty" mapstructure:"block_quote"`
	Paragraph             MarkdownStyle `yaml:"paragraph,omitempty" json:"paragraph,omitempty" mapstructure:"paragraph"`
	List                  MarkdownStyle `yaml:"list,omitempty" json:"list,omitempty" mapstructure:"list"`
	ListItem              MarkdownStyle `yaml:"list_item,omitempty" json:"list_item,omitempty" mapstructure:"list_item"`
	Heading               MarkdownStyle `yaml:"heading,omitempty" json:"heading,omitempty" mapstructure:"heading"`
	H1                    MarkdownStyle `yaml:"h1,omitempty" json:"h1,omitempty" mapstructure:"h1"`
	H2                    MarkdownStyle `yaml:"h2,omitempty" json:"h2,omitempty" mapstructure:"h2"`
	H3                    MarkdownStyle `yaml:"h3,omitempty" json:"h3,omitempty" mapstructure:"h3"`
	H4                    MarkdownStyle `yaml:"h4,omitempty" json:"h4,omitempty" mapstructure:"h4"`
	H5                    MarkdownStyle `yaml:"h5,omitempty" json:"h5,omitempty" mapstructure:"h5"`
	H6                    MarkdownStyle `yaml:"h6,omitempty" json:"h6,omitempty" mapstructure:"h6"`
	Text                  MarkdownStyle `yaml:"text,omitempty" json:"text,omitempty" mapstructure:"text"`
	Strong                MarkdownStyle `yaml:"strong,omitempty" json:"strong,omitempty" mapstructure:"strong"`
	Emph                  MarkdownStyle `yaml:"emph,omitempty" json:"emph,omitempty" mapstructure:"emph"`
	Hr                    MarkdownStyle `yaml:"hr,omitempty" json:"hr,omitempty" mapstructure:"hr"`
	Item                  MarkdownStyle `yaml:"item,omitempty" json:"item,omitempty" mapstructure:"item"`
	Enumeration           MarkdownStyle `yaml:"enumeration,omitempty" json:"enumeration,omitempty" mapstructure:"enumeration"`
	Code                  MarkdownStyle `yaml:"code,omitempty" json:"code,omitempty" mapstructure:"code"`
	CodeBlock             MarkdownStyle `yaml:"code_block,omitempty" json:"code_block,omitempty" mapstructure:"code_block"`
	Table                 MarkdownStyle `yaml:"table,omitempty" json:"table,omitempty" mapstructure:"table"`
	DefinitionList        MarkdownStyle `yaml:"definition_list,omitempty" json:"definition_list,omitempty" mapstructure:"definition_list"`
	DefinitionTerm        MarkdownStyle `yaml:"definition_term,omitempty" json:"definition_term,omitempty" mapstructure:"definition_term"`
	DefinitionDescription MarkdownStyle `yaml:"definition_description,omitempty" json:"definition_description,omitempty" mapstructure:"definition_description"`
	HtmlBlock             MarkdownStyle `yaml:"html_block,omitempty" json:"html_block,omitempty" mapstructure:"html_block"`
	HtmlSpan              MarkdownStyle `yaml:"html_span,omitempty" json:"html_span,omitempty" mapstructure:"html_span"`
	Link                  MarkdownStyle `yaml:"link,omitempty" json:"link,omitempty" mapstructure:"link"`
	LinkText              MarkdownStyle `yaml:"link_text,omitempty" json:"link_text,omitempty" mapstructure:"link_text"`
}

type MarkdownStyle struct {
	BlockPrefix     string                 `yaml:"block_prefix,omitempty" json:"block_prefix,omitempty" mapstructure:"block_prefix"`
	BlockSuffix     string                 `yaml:"block_suffix,omitempty" json:"block_suffix,omitempty" mapstructure:"block_suffix"`
	Color           string                 `yaml:"color,omitempty" json:"color,omitempty" mapstructure:"color"`
	BackgroundColor string                 `yaml:"background_color,omitempty" json:"background_color,omitempty" mapstructure:"background_color"`
	Bold            bool                   `yaml:"bold,omitempty" json:"bold,omitempty" mapstructure:"bold"`
	Italic          bool                   `yaml:"italic,omitempty" json:"italic,omitempty" mapstructure:"italic"`
	Underline       bool                   `yaml:"underline,omitempty" json:"underline,omitempty" mapstructure:"underline"`
	Margin          int                    `yaml:"margin,omitempty" json:"margin,omitempty" mapstructure:"margin"`
	Padding         int                    `yaml:"padding,omitempty" json:"padding,omitempty" mapstructure:"padding"`
	Indent          int                    `yaml:"indent,omitempty" json:"indent,omitempty" mapstructure:"indent"`
	IndentToken     string                 `yaml:"indent_token,omitempty" json:"indent_token,omitempty" mapstructure:"indent_token"`
	LevelIndent     int                    `yaml:"level_indent,omitempty" json:"level_indent,omitempty" mapstructure:"level_indent"`
	Format          string                 `yaml:"format,omitempty" json:"format,omitempty" mapstructure:"format"`
	Prefix          string                 `yaml:"prefix,omitempty" json:"prefix,omitempty" mapstructure:"prefix"`
	StyleOverride   bool                   `yaml:"style_override,omitempty" json:"style_override,omitempty" mapstructure:"style_override"`
	Chroma          map[string]ChromaStyle `yaml:"chroma,omitempty" json:"chroma,omitempty" mapstructure:"chroma"`
}
