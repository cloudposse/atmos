package markdown

// DefaultStyle defines the default Atmos markdown style
var DefaultStyle = []byte(`{
  "document": {
    "block_prefix": "",
    "block_suffix": "\n",
    "color": "#FFFFFF",
    "margin": 0
  },
  "block_quote": {
    "indent": 1,
    "indent_token": "│ ",
    "color": "#9B51E0"
  },
  "paragraph": {
    "block_prefix": "",
    "block_suffix": "",
    "color": "#FFFFFF"
  },
  "list": {
    "level_indent": 4,
    "color": "#FFFFFF",
    "margin": 0,
    "block_suffix": ""
  },
  "list_item": {
    "block_prefix": "– ",
    "color": "#FFFFFF",
    "margin": 0,
    "block_suffix": ""
  },
  "heading": {
    "block_prefix": "",
    "block_suffix": "\n",
    "color": "#00A3E0",
    "bold": true,
    "margin": 0
  },
  "h1": {
    "prefix": "# ",
    "color": "#00A3E0",
    "bold": true,
    "margin": 1
  },
  "h2": {
    "prefix": "## ",
    "color": "#9B51E0",
    "bold": true,
    "margin": 1
  },
  "h3": {
    "prefix": "### ",
    "color": "#00A3E0",
    "bold": true
  },
  "h4": {
    "prefix": "#### ",
    "color": "#00A3E0",
    "bold": true
  },
  "h5": {
    "prefix": "##### ",
    "color": "#00A3E0",
    "bold": true
  },
  "h6": {
    "prefix": "###### ",
    "color": "#00A3E0",
    "bold": true
  },
  "text": {
    "color": "#FFFFFF"
  },
  "strong": {
    "color": "#9B51E0",
    "bold": true
  },
  "emph": {
    "color": "#9B51E0",
    "italic": true
  },
  "hr": {
    "color": "#9B51E0",
    "format": "\n--------\n"
  },
  "item": {
    "block_prefix": "• "
  },
  "enumeration": {
    "block_prefix": ". "
  },
  "code": {
    "color": "#9B51E0"
  },
  "code_block": {
    "margin": 1,
    "indent": 2,
    "block_suffix": "",
    "chroma": {
      "text": {
        "color": "#00A3E0"
      },
      "keyword": {
        "color": "#9B51E0"
      },
      "literal": {
        "color": "#00A3E0"
      },
      "string": {
        "color": "#00A3E0"
      },
      "name": {
        "color": "#00A3E0"
      },
      "number": {
        "color": "#00A3E0"
      },
      "comment": {
        "color": "#9B51E0"
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
    "color": "#00A3E0",
    "underline": true
  },
  "link_text": {
    "color": "#9B51E0",
    "bold": true
  }
}`)
