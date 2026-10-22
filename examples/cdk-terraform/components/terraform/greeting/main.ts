import {
  App, LocalBackend, TerraformOutput, TerraformProvider, TerraformResource, TerraformStack,
  TerraformVariable,
} from 'cdktn';
import { Construct } from 'constructs';

// A minimal AWS provider declaration. Generated provider bindings (`cdktn get`) are the
// usual choice; this generic form keeps the example dependency-free.
class AwsProvider extends TerraformProvider {
  constructor(scope: Construct, id: string, private readonly region: string) {
    super(scope, id, {
      terraformResourceType: 'aws',
      terraformGeneratorMetadata: { providerName: 'aws', providerVersionConstraint: '~> 6.0' },
      terraformProviderSource: 'hashicorp/aws',
    });
  }
  protected synthesizeAttributes(): Record<string, unknown> {
    return { region: this.region };
  }
}

const app = new App();
const stack = new TerraformStack(app, 'Demo');
new LocalBackend(stack, { path: 'terraform.Demo.tfstate' });
const region = new TerraformVariable(stack, 'region', { type: 'string' });
const message = new TerraformVariable(stack, 'message', { type: 'string' });
const provider = new AwsProvider(stack, 'aws', region.stringValue);
const parameter = new TerraformResource(stack, 'greeting', {
  terraformResourceType: 'aws_ssm_parameter',
  provider,
});
parameter.addOverride('name', '/atmos/greeting');
parameter.addOverride('type', 'String');
parameter.addOverride('value', message.stringValue);
new TerraformOutput(stack, 'parameter_name', { value: parameter.getStringAttribute('name') });
app.synth();
