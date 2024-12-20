package markdown

// DefaultStyle defines the default Atmos markdown style
var DefaultStyle = []byte(`{
  "document": {
    "block_prefix": "",
    "block_suffix": "\n",
    "color": "#ffffff",
    "margin": 0
  },
  "block_quote": {
    "indent": 1,
    "indent_token": "│ ",
    "color": "#8a2be2"
  },
  "paragraph": {
    "block_prefix": "",
    "block_suffix": "",
    "color": "#ffffff"
  },
  "list": {
    "level_indent": 2,
    "color": "#ffffff",
    "margin": 0,
    "block_suffix": ""
  },
  "list_item": {
    "block_prefix": "– ",
    "color": "#ffffff",
    "margin": 0,
    "block_suffix": ""
  },
  "heading": {
    "block_prefix": "",
    "block_suffix": "\n",
    "color": "#8a2be2",
    "bold": true,
    "margin": 0
  },
  "h1": {
    "prefix": "# ",
    "color": "#8a2be2",
    "bold": true,
    "margin": 1
  },
  "h2": {
    "prefix": "## ",
    "color": "#8a2be2",
    "bold": true,
    "margin": 1
  },
  "h3": {
    "prefix": "### ",
    "color": "#8a2be2",
    "bold": true
  },
  "h4": {
    "prefix": "#### ",
    "color": "#8a2be2",
    "bold": true
  },
  "h5": {
    "prefix": "##### ",
    "color": "#8a2be2",
    "bold": true
  },
  "h6": {
    "prefix": "###### ",
    "color": "#8a2be2",
    "bold": true
  },
  "text": {
    "color": "#ffffff"
  },
  "strong": {
    "color": "#8a2be2",
    "bold": true
  },
  "emph": {
    "color": "#8a2be2",
    "italic": true
  },
  "hr": {
    "color": "#8a2be2",
    "format": "\n--------\n"
  },
  "item": {
    "block_prefix": "• "
  },
  "enumeration": {
    "block_prefix": ". "
  },
  "code": {
    "color": "#00ffff"
  },
  "code_block": {
    "margin": 0,
    "block_suffix": "",
    "chroma": {
      "text": {
        "color": "#00ffff"
      },
      "keyword": {
        "color": "#8a2be2"
      },
      "literal": {
        "color": "#00ffff"
      },
      "string": {
        "color": "#00ffff"
      },
      "name": {
        "color": "#00ffff"
      },
      "number": {
        "color": "#00ffff"
      },
      "comment": {
        "color": "#8a2be2"
      }
    }
  },
  "table": {
    "center_separator": "┼",
    "column_separator": "│",
    "row_separator": "─"
  },
  "definition_list": {},
  "definition_term": {},
  "definition_description": {
    "block_prefix": "\n"
  },
  "html_block": {},
  "html_span": {},
  "link": {
    "color": "#00ffff",
    "underline": true
  },
  "link_text": {
    "color": "#00ffff",
    "bold": true
  }
}`)
