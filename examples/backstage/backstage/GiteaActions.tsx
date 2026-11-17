import { Button, Grid, Typography } from '@material-ui/core';
import { InfoCard } from '@backstage/core-components';

export const GiteaActions = () => (
  <Grid container spacing={3}>
    <Grid item xs={12}>
      <InfoCard title="Gitea Actions" subheader="Atmos infrastructure delivery">
        <Typography paragraph>
          Service requests open a pull request in atmos/deployments. The plan job
          validates the changed Atmos stacks and previews their Terraform changes.
          After merge, the deploy job provisions the local S3 bucket and registers
          the service in this catalog.
        </Typography>
        <Typography paragraph>
          Open Gitea to see live job status and logs. Sign in with atmos / atmos.
          The payments component illustrates configuration; request a new service
          from Create to trigger this pipeline.
        </Typography>
        <Grid container spacing={2}>
          <Grid item><Button variant="contained" color="primary" href="http://localhost:3001/atmos/deployments/actions">View Actions runs</Button></Grid>
          <Grid item><Button variant="outlined" href="http://localhost:3001/atmos/deployments/pulls">Review pull requests</Button></Grid>
          <Grid item><Button href="http://localhost:3000/docs/default/component/payments/cicd">Read the CI/CD guide</Button></Grid>
        </Grid>
      </InfoCard>
    </Grid>
  </Grid>
);
