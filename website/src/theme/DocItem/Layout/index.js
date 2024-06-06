import React, { useEffect } from 'react';
import Layout from '@theme-original/DocItem/Layout';
import { resetStepCounter } from '@site/src/components/Step';

export default function LayoutWrapper(props) {
  useEffect(() => {
    resetStepCounter(); // Reset the counter whenever the layout is rendered
  }, []);

  return (
    <>
      <Layout {...props} />
    </>
  );
}
