/**
 * Tests for YAML to JSON/HCL converter.
 */
const { convertYamlToFormats, parseYamlWithFunctions, generateJson, generateHcl } = require('./converter');

describe('convertYamlToFormats', () => {
  test('converts simple YAML to all formats', () => {
    const yaml = `settings:
  region: us-west-2
  timeout: 30`;

    const result = convertYamlToFormats(yaml);

    expect(result.yaml).toContain('region: us-west-2');
    expect(result.json).toContain('"region": "us-west-2"');
    expect(result.hcl).toContain('region = "us-west-2"');
  });

  test('converts nested objects', () => {
    const yaml = `database:
  host: localhost
  port: 5432
  credentials:
    username: admin`;

    const result = convertYamlToFormats(yaml);

    expect(result.json).toContain('"host": "localhost"');
    expect(result.json).toContain('"port": 5432');
    expect(result.hcl).toContain('host = "localhost"');
    expect(result.hcl).toContain('port = 5432');
  });

  test('converts arrays', () => {
    const yaml = `tags:
  - production
  - critical`;

    const result = convertYamlToFormats(yaml);

    expect(result.json).toContain('["production", "critical"]');
    expect(result.hcl).toContain('"production"');
    expect(result.hcl).toContain('"critical"');
  });

  test('converts YAML with !env function', () => {
    const yaml = `settings:
  region: !env AWS_REGION`;

    const result = convertYamlToFormats(yaml);

    expect(result.yaml).toContain('!env AWS_REGION');
    expect(result.json).toContain('${env:AWS_REGION}');
    expect(result.hcl).toContain('atmos.env("AWS_REGION")');
  });

  test('converts YAML with !exec function', () => {
    const yaml = `version: !exec "git describe --tags"`;

    const result = convertYamlToFormats(yaml);

    expect(result.json).toContain('${exec:git describe --tags}');
    expect(result.hcl).toContain('atmos.exec("git describe --tags")');
  });

  test('converts YAML with !terraform.output function', () => {
    const yaml = `vpc_id: !terraform.output vpc.id stack=prod-us-west-2`;

    const result = convertYamlToFormats(yaml);

    expect(result.json).toContain('${terraform.output:vpc.id stack=prod-us-west-2}');
    expect(result.hcl).toContain('atmos.terraform_output');
  });

  test('converts YAML with !store function', () => {
    const yaml = `api_key: !store ssm/api/key`;

    const result = convertYamlToFormats(yaml);

    expect(result.json).toContain('${store:ssm/api/key}');
    expect(result.hcl).toContain('atmos.store("ssm", "api/key")');
  });

  test('preserves boolean values', () => {
    const yaml = `enabled: true
disabled: false`;

    const result = convertYamlToFormats(yaml);

    expect(result.json).toContain('"enabled": true');
    expect(result.json).toContain('"disabled": false');
    expect(result.hcl).toContain('enabled = true');
    expect(result.hcl).toContain('disabled = false');
  });

  test('preserves null values', () => {
    const yaml = `optional: null`;

    const result = convertYamlToFormats(yaml);

    expect(result.json).toContain('"optional": null');
    expect(result.hcl).toContain('optional = null');
  });
});

describe('parseYamlWithFunctions', () => {
  test('parses standard YAML', () => {
    const yaml = `key: value`;
    const result = parseYamlWithFunctions(yaml);

    expect(result).toEqual({ key: 'value' });
  });

  test('parses YAML with !env tag', () => {
    const yaml = `region: !env AWS_REGION`;
    const result = parseYamlWithFunctions(yaml);

    expect(result.region.__atmosFunction).toBe('env');
    expect(result.region.__atmosArg).toBe('AWS_REGION');
  });

  test('parses YAML with !template tag', () => {
    const yaml = `config: !template "{{ .Values.name }}"`;
    const result = parseYamlWithFunctions(yaml);

    expect(result.config.__atmosFunction).toBe('template');
    expect(result.config.__atmosArg).toBe('{{ .Values.name }}');
  });
});

describe('generateJson', () => {
  test('generates formatted JSON', () => {
    const data = { key: 'value', nested: { inner: 123 } };
    const result = generateJson(data);

    expect(result).toBe(JSON.stringify(data, null, 2));
  });
});

describe('generateHcl', () => {
  test('generates HCL for simple object', () => {
    const data = { region: 'us-west-2' };
    const result = generateHcl(data);

    expect(result).toContain('region = "us-west-2"');
    expect(result).toContain('{');
    expect(result).toContain('}');
  });

  test('generates HCL for nested objects', () => {
    const data = {
      settings: {
        timeout: 30,
        enabled: true,
      },
    };
    const result = generateHcl(data);

    expect(result).toContain('settings = {');
    expect(result).toContain('timeout = 30');
    expect(result).toContain('enabled = true');
  });
});
