/** Observed JSON fields, not an exhaustive schema or inferred optionality. */
function inputFields(request) {
  const fields = [];
  function visit(value, path) {
    const type = value === null ? 'null' : Array.isArray(value) ? 'array' : typeof value;
    fields.push({path, type});
    if (Array.isArray(value)) {
      value.forEach((item, index) => visit(item, `${path}[${index}]`));
    } else if (value !== null && typeof value === 'object') {
      Object.keys(value).sort().forEach(key => visit(value[key], `${path}[${JSON.stringify(key)}]`));
    }
  }
  visit(request.input, 'input');
  visit(request.ci ?? null, 'ci');
  const metadata = Object.fromEntries(['kind', 'revisions', 'subject']
    .filter(key => Object.hasOwn(request, key)).map(key => [key, request[key]]));
  if (Object.keys(metadata).length) visit(metadata, 'request');
  return fields;
}

module.exports = {inputFields};
