/**
 * YAML to JSON/HCL Converter with Atmos function syntax translation.
 *
 * Translates Atmos YAML function syntax to equivalent formats:
 * - YAML: !env VAR, !template "...", !exec "...", !terraform.output, !store, etc.
 * - JSON: ${env:VAR}, ${template:...}, ${exec:...}, ${terraform.output:...}, ${store:...}
 * - HCL: atmos::env("VAR"), atmos::template("..."), atmos::exec("..."), etc.
 *
 * HCL output uses proper block syntax:
 * - Nested objects use block notation: key { }
 * - Components use labeled blocks: component "name" { }
 * - Full stacks wrap in: stack "name" { } or stack { }
 */
const yaml = require('js-yaml');
const { translateFunctions } = require('./function-syntax');

/**
 * Convert YAML string to all supported formats.
 * Supports multi-document YAML (separated by ---).
 * @param {string} yamlStr - The YAML source string.
 * @returns {{ yaml: string, json: string, hcl: string }} - Converted formats.
 */
function convertYamlToFormats(yamlStr) {
  // Check for multi-document YAML.
  const documents = splitYamlDocuments(yamlStr);

  if (documents.length > 1) {
    // Multi-document: generate multiple stack blocks.
    const parsedDocs = documents.map((doc) => parseYamlWithFunctions(doc));

    return {
      yaml: normalizeYaml(yamlStr),
      json: generateMultiDocJson(parsedDocs),
      hcl: generateMultiDocHcl(parsedDocs),
    };
  }

  // Single document.
  const parsed = parseYamlWithFunctions(yamlStr);

  return {
    yaml: normalizeYaml(yamlStr),
    json: generateJson(parsed),
    hcl: generateHcl(parsed),
  };
}

/**
 * Split YAML string into separate documents.
 * @param {string} yamlStr - The YAML source string.
 * @returns {string[]} - Array of YAML document strings.
 */
function splitYamlDocuments(yamlStr) {
  // Split on document separators (--- at start of line).
  const docs = yamlStr.split(/^---$/m).filter((doc) => doc.trim());
  return docs;
}

/**
 * Parse YAML with custom Atmos function tags.
 * Returns an AST-like structure that preserves function information.
 *
 * Uses regex-based extraction since js-yaml 4.x custom types are complex
 * and we only need to identify function calls for documentation purposes.
 */
function parseYamlWithFunctions(yamlStr) {
  // List of all Atmos functions to extract.
  const atmosFunctions = [
    'env',
    'template',
    'exec',
    'repo-root',
    'terraform.output',
    'terraform.state',
    'store',
    'store.get',
    'include',
    'include.raw',
    'random',
    'aws.account_id',
    'aws.region',
    'aws.caller_identity_arn',
    'aws.caller_identity_user_id',
  ];
  const functionMarkers = [];
  let processedYaml = yamlStr;

  // Match YAML function syntax: !funcname args
  // Handle both simple (!env VAR) and quoted (!env 'VAR "default"') forms.
  // Escape all regex special characters in function names for safety.
  const escapeRegex = (str) => str.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');

  atmosFunctions.forEach((funcName) => {
    const escapedName = escapeRegex(funcName);

    // Functions without arguments (like !repo-root, !random).
    const noArgRegex = new RegExp(`!${escapedName}(?=\\s*$|\\s*\\n)`, 'gm');
    processedYaml = processedYaml.replace(noArgRegex, () => {
      const marker = `__ATMOS_FN_${functionMarkers.length}__`;
      functionMarkers.push({
        marker,
        funcName,
        args: '',
      });
      return `"${marker}"`;
    });

    // Functions with arguments.
    const regex = new RegExp(`!${escapedName}\\s+(.+?)(?=\\n|$)`, 'g');
    processedYaml = processedYaml.replace(regex, (match, args) => {
      const marker = `__ATMOS_FN_${functionMarkers.length}__`;
      functionMarkers.push({
        marker,
        funcName,
        args: args.trim(),
      });
      return `"${marker}"`;
    });
  });

  try {
    // Parse the processed YAML (functions replaced with string markers).
    const parsed = yaml.load(processedYaml);

    // Restore function markers to function objects.
    return restoreFunctionMarkers(parsed, functionMarkers);
  } catch (err) {
    // If parsing fails, try without function extraction.
    console.warn(`[converter] YAML parse warning: ${err.message}`);
    try {
      return yaml.load(yamlStr);
    } catch {
      // Return empty object if all parsing fails.
      return {};
    }
  }
}

/**
 * Recursively restore function markers to function objects.
 */
function restoreFunctionMarkers(data, markers) {
  if (data === null || data === undefined) {
    return data;
  }

  if (typeof data === 'string') {
    // Check if this is a function marker.
    const marker = markers.find((m) => data === m.marker);
    if (marker) {
      return {
        __atmosFunction: marker.funcName,
        __atmosArg: marker.args,
      };
    }
    return data;
  }

  if (Array.isArray(data)) {
    return data.map((item) => restoreFunctionMarkers(item, markers));
  }

  if (typeof data === 'object') {
    const result = {};
    for (const [key, value] of Object.entries(data)) {
      result[key] = restoreFunctionMarkers(value, markers);
    }
    return result;
  }

  return data;
}

/**
 * Generate JSON with translated function syntax.
 */
function generateJson(data) {
  const translated = translateFunctions(data, 'json');
  return JSON.stringify(translated, null, 2);
}

/**
 * Generate JSON for multiple documents.
 * @param {Object[]} documents - Array of parsed YAML documents.
 * @returns {string} - JSON array string.
 */
function generateMultiDocJson(documents) {
  const translated = documents.map((doc) => translateFunctions(doc, 'json'));
  return JSON.stringify(translated, null, 2);
}

/**
 * Generate HCL with translated function syntax.
 * Uses proper HCL block syntax with stack wrapper and component labels.
 */
function generateHcl(data) {
  const translated = translateFunctions(data, 'hcl');

  // Detect if this is a full stack (has components or explicit name).
  const isFullStack = 'components' in translated || 'name' in translated;

  if (isFullStack) {
    // Extract stack name for block label.
    const stackName = translated.name || null;
    return writeStackHcl(translated, stackName);
  }

  // Partial snippet - no stack wrapper.
  return writeBodyHcl(translated, 0).trim();
}

/**
 * Generate HCL for multiple documents.
 * @param {Object[]} documents - Array of parsed YAML documents.
 * @returns {string} - HCL string with multiple stack blocks.
 */
function generateMultiDocHcl(documents) {
  const hclParts = documents.map((doc) => {
    const translated = translateFunctions(doc, 'hcl');
    const stackName = translated.name || null;
    return writeStackHcl(translated, stackName);
  });

  return hclParts.join('\n');
}

/**
 * Write a stack block with optional name label.
 * @param {Object} data - Stack configuration data.
 * @param {string|null} stackName - Optional stack name for block label.
 * @returns {string} - HCL stack block.
 */
function writeStackHcl(data, stackName) {
  const label = stackName ? ` "${stackName}"` : '';
  let result = `stack${label} {\n`;

  // Write body, excluding 'name' field (it's the block label).
  const bodyData = { ...data };
  delete bodyData.name;

  result += writeBodyHcl(bodyData, 1);
  result += '}\n';
  return result;
}

/**
 * Write HCL body content with proper block syntax.
 * @param {Object} obj - Object to convert.
 * @param {number} indent - Current indentation level.
 * @returns {string} - HCL body content.
 */
function writeBodyHcl(obj, indent) {
  if (!obj || typeof obj !== 'object') {
    return '';
  }

  const spaces = '  '.repeat(indent);
  let result = '';

  for (const [key, value] of Object.entries(obj)) {
    if (key === 'components') {
      // Special handling for components section.
      result += writeComponentsHcl(value, indent);
    } else if (isPlainObject(value)) {
      // Block syntax for nested objects.
      result += `${spaces}${key} {\n`;
      result += writeBodyHcl(value, indent + 1);
      result += `${spaces}}\n`;
    } else {
      // Attribute syntax for primitives and arrays.
      result += `${spaces}${key} = ${formatHclValue(value, indent)}\n`;
    }
  }

  return result;
}

/**
 * Write components section with labeled component blocks.
 * @param {Object} components - Components object with terraform/helmfile sections.
 * @param {number} indent - Current indentation level.
 * @returns {string} - HCL components block.
 */
function writeComponentsHcl(components, indent) {
  if (!components || typeof components !== 'object') {
    return '';
  }

  const spaces = '  '.repeat(indent);
  let result = `\n${spaces}components {\n`;

  for (const [type, typeComponents] of Object.entries(components)) {
    if (!typeComponents || typeof typeComponents !== 'object') {
      continue;
    }

    result += `${spaces}  ${type} {\n`;

    for (const [name, config] of Object.entries(typeComponents)) {
      // Labeled block: component "name" { }.
      result += `${spaces}    component "${name}" {\n`;
      if (config && typeof config === 'object') {
        result += writeBodyHcl(config, indent + 3);
      }
      result += `${spaces}    }\n`;
    }

    result += `${spaces}  }\n`;
  }

  result += `${spaces}}\n`;
  return result;
}

/**
 * Format a value for HCL output.
 * @param {any} value - Value to format.
 * @param {number} indent - Current indentation level.
 * @returns {string} - Formatted HCL value.
 */
function formatHclValue(value, indent) {
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
    if (value.startsWith('atmos::') || value.startsWith('atmos_')) {
      return value;
    }
    // Escape special characters in HCL strings.
    // Order matters: escape backslashes first, then other sequences.
    const escaped = value
      .replace(/\\/g, '\\\\')
      .replace(/"/g, '\\"')
      .replace(/\n/g, '\\n')
      .replace(/\r/g, '\\r')
      .replace(/\t/g, '\\t')
      .replace(/\${/g, '$${'); // Escape HCL interpolation sequences.
    return `"${escaped}"`;
  }

  if (Array.isArray(value)) {
    if (value.length === 0) {
      return '[]';
    }
    const items = value.map((item) => formatHclValue(item, indent + 1));
    // Simple arrays on single line, complex on multiple lines.
    const allPrimitives = value.every((v) => typeof v !== 'object' || v === null);
    if (allPrimitives && items.join(', ').length < 60) {
      return `[${items.join(', ')}]`;
    }
    return `[\n${spaces}  ${items.join(`,\n${spaces}  `)},\n${spaces}]`;
  }

  if (typeof value === 'object') {
    const entries = Object.entries(value);
    if (entries.length === 0) {
      return '{}';
    }
    const lines = entries.map(([k, v]) => `${spaces}  ${k} = ${formatHclValue(v, indent + 1)}`);
    return `{\n${lines.join('\n')}\n${spaces}}`;
  }

  return String(value);
}

/**
 * Check if a value is a plain object (not array, null, or other).
 */
function isPlainObject(value) {
  return value !== null && typeof value === 'object' && !Array.isArray(value);
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

// Legacy export for backwards compatibility.
function objectToHcl(obj, indent = 0) {
  // For backwards compatibility, delegate to new implementation.
  return formatHclValue(obj, indent);
}

module.exports = {
  convertYamlToFormats,
  parseYamlWithFunctions,
  generateJson,
  generateHcl,
  generateMultiDocHcl,
  objectToHcl,
  writeStackHcl,
  writeBodyHcl,
  writeComponentsHcl,
  formatHclValue,
};
