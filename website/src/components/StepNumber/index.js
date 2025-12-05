import React from 'react';
import { MdLooksOne, MdLooksTwo, MdLooks3, MdLooks4, MdLooks5, MdLooks6 } from 'react-icons/md';
import './index.css';

const iconMap = {
  "1": MdLooksOne,
  "2": MdLooksTwo,
  "3": MdLooks3,
  "4": MdLooks4,
  "5": MdLooks5,
  "6": MdLooks6
};

const StepNumber = ({ step, children }) => {
  const Icon = iconMap[step];
  if (!Icon) return null;

  return (
    <span className="step-number">
      <Icon className="step-number-icon" />
      {children}
    </span>
  );
};

export default StepNumber;
