/**
 * Atmos Function Syntax Translation.
 *
 * Translates Atmos function objects to target format syntax:
 *
 * Format translations:
 * | YAML                    | JSON                         | HCL                                    |
 * |-------------------------|------------------------------|----------------------------------------|
 * | !env VAR                | ${env:VAR}                   | atmos.env("VAR")                       |
 * | !template "..."         | ${template:...}              | atmos.template("...")                  |
 * | !exec "cmd"             | ${exec:cmd}                  | atmos.exec("cmd")                      |
 * | !repo-root              | ${repo-root}                 | atmos.repo_root()                      |
 * | !terraform.output ...   | ${terraform.output:...}      | atmos.terraform_output(...)            |
 * | !terraform.state ...    | ${terraform.state:...}       | atmos.terraform_state(...)             |
 * | !store provider/key     | ${store:provider/key}        | atmos.store("provider", "key")         |
 */

/**
 * Translate function objects to target format.
 * @param {any} data - Parsed YAML data with function objects.
 * @param {string} format - Target format ('json' or 'hcl').
 * @returns {any} - Data with translated function strings.
 */
function translateFunctions(data, format) {
  if (data === null || data === undefined) {
    return data;
  }

  // Check if this is an Atmos function object.
  if (isAtmosFunction(data)) {
    return translateFunction(data.__atmosFunction, data.__atmosArg, format);
  }

  // Recursively process arrays.
  if (Array.isArray(data)) {
    return data.map((item) => translateFunctions(item, format));
  }

  // Recursively process objects.
  if (typeof data === 'object') {
    const result = {};
    for (const [key, value] of Object.entries(data)) {
      result[key] = translateFunctions(value, format);
    }
    return result;
  }

  // Return primitives as-is.
  return data;
}

/**
 * Check if an object is an Atmos function marker.
 */
function isAtmosFunction(obj) {
  return obj !== null && typeof obj === 'object' && '__atmosFunction' in obj;
}

/**
 * Translate a single function to target format.
 */
function translateFunction(funcName, arg, format) {
  if (format === 'json') {
    return translateToJson(funcName, arg);
  }
  if (format === 'hcl') {
    return translateToHcl(funcName, arg);
  }
  // Default: return YAML-style for unknown formats.
  return `!${funcName} ${arg || ''}`.trim();
}

/**
 * Translate function to JSON interpolation syntax.
 */
function translateToJson(funcName, arg) {
  if (funcName === 'repo-root') {
    return '${repo-root}';
  }

  if (!arg) {
    return `\${${funcName}}`;
  }

  return `\${${funcName}:${arg}}`;
}

/**
 * Translate function to HCL function call syntax.
 * Uses namespaced format: atmos.func_name()
 */
function translateToHcl(funcName, arg) {
  // Normalize function name for HCL (replace - with _, keep . for terraform functions).
  // Result: atmos.env, atmos.exec, atmos.repo_root, atmos.terraform_output, etc.
  const normalizedName = funcName.replace(/-/g, '_').replace(/\./g, '_');
  const hclFuncName = `atmos.${normalizedName}`;

  if (funcName === 'repo-root') {
    return `${hclFuncName}()`;
  }

  if (funcName === 'store') {
    // Parse store argument: provider/key -> atmos_store("provider", "key").
    const parts = (arg || '').split('/');
    if (parts.length >= 2) {
      const provider = parts[0];
      const key = parts.slice(1).join('/');
      return `${hclFuncName}("${provider}", "${key}")`;
    }
    return `${hclFuncName}("${arg || ''}")`;
  }

  if (funcName === 'terraform.output' || funcName === 'terraform.state') {
    // Parse terraform function arguments.
    // Format: component.output_name stack=stack-name
    const parsed = parseTerraformArg(arg || '');
    const args = [];
    if (parsed.component) args.push(`"${parsed.component}"`);
    if (parsed.output) args.push(`"${parsed.output}"`);
    if (parsed.stack) args.push(`stack = "${parsed.stack}"`);
    return `${hclFuncName}(${args.join(', ')})`;
  }

  if (funcName === 'env') {
    // Environment variable: !env VAR -> atmos_env("VAR").
    return `${hclFuncName}("${arg || ''}")`;
  }

  if (funcName === 'template') {
    // Template: !template "..." -> atmos_template("...").
    const templateArg = (arg || '').replace(/^["']|["']$/g, '');
    return `${hclFuncName}("${templateArg}")`;
  }

  if (funcName === 'exec') {
    // Exec: !exec "cmd" -> atmos_exec("cmd").
    const execArg = (arg || '').replace(/^["']|["']$/g, '');
    return `${hclFuncName}("${execArg}")`;
  }

  // Default: generic function call.
  if (arg) {
    return `${hclFuncName}("${arg}")`;
  }
  return `${hclFuncName}()`;
}

/**
 * Parse Terraform function argument.
 * Format: component.output_name stack=stack-name
 */
function parseTerraformArg(arg) {
  const result = { component: '', output: '', stack: '' };

  // Extract stack= parameter if present.
  const stackMatch = arg.match(/\s+stack=(\S+)/);
  if (stackMatch) {
    result.stack = stackMatch[1];
    arg = arg.replace(stackMatch[0], '').trim();
  }

  // Parse component.output.
  const parts = arg.split('.');
  if (parts.length >= 2) {
    result.component = parts[0];
    result.output = parts.slice(1).join('.');
  } else if (parts.length === 1) {
    result.component = parts[0];
  }

  return result;
}

module.exports = {
  translateFunctions,
  translateFunction,
  translateToJson,
  translateToHcl,
  isAtmosFunction,
};
