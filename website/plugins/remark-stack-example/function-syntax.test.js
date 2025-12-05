/**
 * Tests for Atmos function syntax translation.
 */
const {
  translateFunctions,
  translateFunction,
  translateToJson,
  translateToHcl,
  isAtmosFunction,
} = require('./function-syntax');

describe('isAtmosFunction', () => {
  test('returns true for Atmos function objects', () => {
    const fn = { __atmosFunction: 'env', __atmosArg: 'AWS_REGION' };
    expect(isAtmosFunction(fn)).toBe(true);
  });

  test('returns false for regular objects', () => {
    expect(isAtmosFunction({ key: 'value' })).toBe(false);
    expect(isAtmosFunction(null)).toBe(false);
    expect(isAtmosFunction('string')).toBe(false);
    expect(isAtmosFunction(123)).toBe(false);
  });
});

describe('translateToJson', () => {
  test('translates env function', () => {
    expect(translateToJson('env', 'AWS_REGION')).toBe('${env:AWS_REGION}');
  });

  test('translates exec function', () => {
    expect(translateToJson('exec', 'git describe')).toBe('${exec:git describe}');
  });

  test('translates template function', () => {
    expect(translateToJson('template', '{{ .name }}')).toBe('${template:{{ .name }}}');
  });

  test('translates repo-root function', () => {
    expect(translateToJson('repo-root', null)).toBe('${repo-root}');
    expect(translateToJson('repo-root', '')).toBe('${repo-root}');
  });

  test('translates terraform.output function', () => {
    expect(translateToJson('terraform.output', 'vpc.id stack=prod')).toBe(
      '${terraform.output:vpc.id stack=prod}'
    );
  });

  test('translates store function', () => {
    expect(translateToJson('store', 'ssm/api/key')).toBe('${store:ssm/api/key}');
  });
});

describe('translateToHcl', () => {
  test('translates env function', () => {
    expect(translateToHcl('env', 'AWS_REGION')).toBe('atmos.env("AWS_REGION")');
  });

  test('translates exec function', () => {
    expect(translateToHcl('exec', 'git describe')).toBe('atmos.exec("git describe")');
  });

  test('translates template function', () => {
    expect(translateToHcl('template', '{{ .name }}')).toBe('atmos.template("{{ .name }}")');
  });

  test('translates repo-root function', () => {
    expect(translateToHcl('repo-root', null)).toBe('atmos.repo_root()');
    expect(translateToHcl('repo-root', '')).toBe('atmos.repo_root()');
  });

  test('translates terraform.output function', () => {
    const result = translateToHcl('terraform.output', 'vpc.id stack=prod');
    expect(result).toContain('atmos.terraform_output');
    expect(result).toContain('"vpc"');
    expect(result).toContain('"id"');
    expect(result).toContain('stack = "prod"');
  });

  test('translates terraform.state function', () => {
    const result = translateToHcl('terraform.state', 'network.cidr_block');
    expect(result).toContain('atmos.terraform_state');
    expect(result).toContain('"network"');
    expect(result).toContain('"cidr_block"');
  });

  test('translates store function with provider/key format', () => {
    expect(translateToHcl('store', 'ssm/api/key')).toBe('atmos.store("ssm", "api/key")');
    expect(translateToHcl('store', 'vault/secrets/db')).toBe('atmos.store("vault", "secrets/db")');
  });
});

describe('translateFunction', () => {
  test('translates to json format', () => {
    expect(translateFunction('env', 'VAR', 'json')).toBe('${env:VAR}');
  });

  test('translates to hcl format', () => {
    expect(translateFunction('env', 'VAR', 'hcl')).toBe('atmos.env("VAR")');
  });

  test('returns YAML style for unknown format', () => {
    const result = translateFunction('env', 'VAR', 'unknown');
    expect(result).toBe('!env VAR');
  });
});

describe('translateFunctions', () => {
  test('preserves primitives', () => {
    expect(translateFunctions('string', 'json')).toBe('string');
    expect(translateFunctions(123, 'json')).toBe(123);
    expect(translateFunctions(true, 'json')).toBe(true);
    expect(translateFunctions(null, 'json')).toBe(null);
  });

  test('translates function objects', () => {
    const fn = { __atmosFunction: 'env', __atmosArg: 'AWS_REGION' };
    expect(translateFunctions(fn, 'json')).toBe('${env:AWS_REGION}');
    expect(translateFunctions(fn, 'hcl')).toBe('atmos.env("AWS_REGION")');
  });

  test('translates nested objects', () => {
    const data = {
      settings: {
        region: { __atmosFunction: 'env', __atmosArg: 'AWS_REGION' },
        timeout: 30,
      },
    };

    const result = translateFunctions(data, 'json');
    expect(result.settings.region).toBe('${env:AWS_REGION}');
    expect(result.settings.timeout).toBe(30);
  });

  test('translates arrays', () => {
    const data = [
      { __atmosFunction: 'env', __atmosArg: 'VAR1' },
      { __atmosFunction: 'env', __atmosArg: 'VAR2' },
      'static',
    ];

    const result = translateFunctions(data, 'json');
    expect(result[0]).toBe('${env:VAR1}');
    expect(result[1]).toBe('${env:VAR2}');
    expect(result[2]).toBe('static');
  });

  test('handles deeply nested structures', () => {
    const data = {
      level1: {
        level2: {
          level3: {
            value: { __atmosFunction: 'exec', __atmosArg: 'cmd' },
          },
        },
      },
    };

    const result = translateFunctions(data, 'hcl');
    expect(result.level1.level2.level3.value).toBe('atmos.exec("cmd")');
  });
});
