/**
 * YAML to JSON/HCL Converter with Atmos function syntax translation.
 *
 * Translates Atmos YAML function syntax to equivalent formats:
 * - YAML: !env VAR, !template "...", !exec "...", !terraform.output, !store
 * - JSON: ${env:VAR}, ${template:...}, ${exec:...}, ${terraform.output:...}, ${store:...}
 * - HCL: atmos_env("VAR"), atmos_template("..."), atmos_exec("..."), atmos_terraform_output(...), atmos_store(...)
 */
const yaml = require('js-yaml');
const { translateFunctions } = require('./function-syntax');

/**
 * Convert YAML string to all supported formats.
 * @param {string} yamlStr - The YAML source string.
 * @returns {{ yaml: string, json: string, hcl: string }} - Converted formats.
 */
function convertYamlToFormats(yamlStr) {
  // Parse YAML with custom tags to preserve function syntax.
  const parsed = parseYamlWithFunctions(yamlStr);

  // Generate each format.
  const jsonOutput = generateJson(parsed);
  const hclOutput = generateHcl(parsed);

  // Clean YAML for display (normalize formatting).
  const yamlOutput = normalizeYaml(yamlStr);

  return {
    yaml: yamlOutput,
    json: jsonOutput,
    hcl: hclOutput,
  };
}

/**
 * Parse YAML with custom Atmos function tags.
 * Returns an AST-like structure that preserves function information.
 */
function parseYamlWithFunctions(yamlStr) {
  // Define custom YAML types for Atmos functions.
  const atmosFunctions = ['env', 'template', 'exec', 'repo-root', 'terraform.output', 'terraform.state', 'store'];

  const customTypes = atmosFunctions.map((funcName) => {
    return new yaml.Type(`!${funcName}`, {
      kind: 'scalar',
      construct: (data) => ({
        __atmosFunction: funcName,
        __atmosArg: data,
      }),
    });
  });

  const schema = yaml.DEFAULT_SCHEMA.extend(customTypes);

  try {
    return yaml.load(yamlStr, { schema });
  } catch (err) {
    // If parsing fails, try simpler approach.
    console.warn(`[converter] YAML parse warning: ${err.message}`);
    return yaml.load(yamlStr);
  }
}

/**
 * Generate JSON with translated function syntax.
 */
function generateJson(data) {
  const translated = translateFunctions(data, 'json');
  return JSON.stringify(translated, null, 2);
}

/**
 * Generate HCL with translated function syntax.
 */
function generateHcl(data) {
  const translated = translateFunctions(data, 'hcl');
  return objectToHcl(translated, 0);
}

/**
 * Convert JavaScript object to HCL format.
 */
function objectToHcl(obj, indent = 0) {
  const spaces = '  '.repeat(indent);
  const lines = [];

  if (Array.isArray(obj)) {
    lines.push('[');
    obj.forEach((item, index) => {
      const value = valueToHcl(item, indent + 1);
      const comma = index < obj.length - 1 ? ',' : '';
      lines.push(`${spaces}  ${value}${comma}`);
    });
    lines.push(`${spaces}]`);
    return lines.join('\n');
  }

  if (typeof obj === 'object' && obj !== null) {
    lines.push('{');
    const keys = Object.keys(obj);
    keys.forEach((key, index) => {
      const value = valueToHcl(obj[key], indent + 1);
      lines.push(`${spaces}  ${key} = ${value}`);
    });
    lines.push(`${spaces}}`);
    return lines.join('\n');
  }

  return valueToHcl(obj, indent);
}

/**
 * Convert a single value to HCL format.
 */
function valueToHcl(value, indent) {
  const spaces = '  '.repeat(indent);

  if (value === null || value === undefined) {
    return 'null';
  }

  if (typeof value === 'boolean') {
    return value ? 'true' : 'false';
  }

  if (typeof value === 'number') {
    return String(value);
  }

  if (typeof value === 'string') {
    // Check if it's an HCL function call (already translated).
    if (value.startsWith('atmos_')) {
      return value;
    }
    // Escape special characters in strings.
    const escaped = value.replace(/\\/g, '\\\\').replace(/"/g, '\\"');
    return `"${escaped}"`;
  }

  if (Array.isArray(value)) {
    if (value.length === 0) {
      return '[]';
    }
    const items = value.map((item) => valueToHcl(item, indent + 1));
    return `[\n${spaces}  ${items.join(`,\n${spaces}  `)}\n${spaces}]`;
  }

  if (typeof value === 'object') {
    const entries = Object.entries(value);
    if (entries.length === 0) {
      return '{}';
    }
    const lines = entries.map(([k, v]) => `${spaces}  ${k} = ${valueToHcl(v, indent + 1)}`);
    return `{\n${lines.join('\n')}\n${spaces}}`;
  }

  return String(value);
}

/**
 * Normalize YAML for display.
 */
function normalizeYaml(yamlStr) {
  // Trim trailing whitespace and ensure consistent line endings.
  return yamlStr
    .split('\n')
    .map((line) => line.trimEnd())
    .join('\n')
    .trim();
}

module.exports = {
  convertYamlToFormats,
  parseYamlWithFunctions,
  generateJson,
  generateHcl,
  objectToHcl,
};
