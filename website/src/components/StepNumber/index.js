import React, { useContext } from 'react';
import { StepContext } from '@site/src/components/Step';

const StepNumber = () => {
  const stepNumber = useContext(StepContext);
  return `Step ${stepNumber}: `;
};

export default StepNumber;
