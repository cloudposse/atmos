import {
  App, BootstraplessSynthesizer, CfnOutput, CfnParameter, CfnResource, Stack,
} from 'aws-cdk-lib';

const app = new App();
const stack = new Stack(app, 'Demo', {
  synthesizer: new BootstraplessSynthesizer(),
});
const message = new CfnParameter(stack, 'Message', { type: 'String' });
const marker = new CfnResource(stack, 'Marker', {
  type: 'AWS::SSM::Parameter',
  properties: { Type: 'String', Value: message.valueAsString },
});
new CfnOutput(stack, 'ParameterName', { value: marker.ref });
app.synth();
