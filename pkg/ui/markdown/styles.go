package markdown

// DefaultStyle defines the default Atmos markdown style
var DefaultStyle = []byte(`{
  "document": {
    "block_prefix": "\n",
    "block_suffix": "\n",
    "color": "#4A5568",
    "margin": 0
  },
  "block_quote": {
    "indent": 1,
    "indent_token": "│ "
  },
  "paragraph": {
    "block_suffix": "\n"
  },
  "list": {
    "level_indent": 2
  },
  "heading": {
    "block_suffix": "\n",
    "color": "#00A3E0",
    "bold": true
  },
  "h1": {
    "prefix": "# ",
    "color": "#00A3E0",
    "bold": true
  },
  "h2": {
    "prefix": "## ",
    "color": "#00A3E0",
    "bold": true
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
  "text": {},
  "strikethrough": {
    "crossed_out": true
  },
  "emph": {
    "italic": true
  },
  "strong": {
    "bold": true
  },
  "hr": {
    "color": "#CBD5E0",
    "format": "\n--------\n"
  },
  "item": {
    "block_prefix": "• "
  },
  "enumeration": {
    "block_prefix": ". "
  },
  "task": {
    "ticked": "[✓] ",
    "unticked": "[ ] "
  },
  "link": {
    "color": "#4299E1",
    "underline": true
  },
  "link_text": {
    "color": "#4299E1"
  },
  "image": {
    "color": "#4299E1"
  },
  "image_text": {
    "color": "#4299E1",
    "format": "Image: {{.text}} →"
  },
  "code": {
    "color": "#4A5568",
    "background_color": "#F7FAFC"
  },
  "code_block": {
    "color": "#4A5568",
    "background_color": "#F7FAFC",
    "margin": 2,
    "chroma": {
      "text": {
        "color": "#4A5568"
      },
      "error": {
        "color": "#F56565",
        "background_color": "#F7FAFC"
      },
      "comment": {
        "color": "#718096"
      },
      "comment_preproc": {
        "color": "#4299E1"
      },
      "keyword": {
        "color": "#00A3E0"
      },
      "keyword_reserved": {
        "color": "#00A3E0"
      },
      "keyword_namespace": {
        "color": "#00A3E0"
      },
      "keyword_type": {
        "color": "#48BB78"
      },
      "operator": {
        "color": "#4A5568"
      },
      "punctuation": {
        "color": "#4A5568"
      },
      "name": {
        "color": "#4A5568"
      },
      "name_builtin": {
        "color": "#00A3E0"
      },
      "name_tag": {
        "color": "#00A3E0"
      },
      "name_attribute": {
        "color": "#48BB78"
      },
      "name_class": {
        "color": "#48BB78"
      },
      "name_constant": {
        "color": "#4299E1"
      },
      "name_decorator": {
        "color": "#4299E1"
      },
      "name_exception": {
        "color": "#F56565"
      },
      "name_function": {
        "color": "#4299E1"
      },
      "name_other": {
        "color": "#4A5568"
      },
      "literal": {
        "color": "#ECC94B"
      },
      "literal_number": {
        "color": "#ECC94B"
      },
      "literal_date": {
        "color": "#ECC94B"
      },
      "literal_string": {
        "color": "#48BB78"
      },
      "literal_string_escape": {
        "color": "#4299E1"
      },
      "generic_deleted": {
        "color": "#F56565"
      },
      "generic_emph": {
        "italic": true
      },
      "generic_inserted": {
        "color": "#48BB78"
      },
      "generic_strong": {
        "bold": true
      },
      "generic_subheading": {
        "color": "#4299E1"
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
    "block_prefix": "\n🠶 "
  },
  "html_block": {},
  "html_span": {}
}`)
