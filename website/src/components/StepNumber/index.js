import React, { useContext } from 'react';
import { StepContext } from '@site/src/components/Step';

const StepNumber = () => {
  const stepNumber = useContext(StepContext);
  return (<i>{`Step ${stepNumber}:`}</i>);
};

export default StepNumber;
