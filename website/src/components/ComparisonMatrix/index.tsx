import React, { useState } from 'react';
import { RiCheckLine } from 'react-icons/ri';
import ToolDrawer, { type Tool } from '../ToolDrawer';
import CapabilityDrawer from '../CapabilityDrawer';
import { toolCategories, allTools, capabilityInfo, type CapabilityInfo } from '@site/src/data/tools';
import './index.css';

const ComparisonMatrix: React.FC = () => {
  const [selectedTool, setSelectedTool] = useState<Tool | null>(null);
  const [isToolDrawerOpen, setIsToolDrawerOpen] = useState(false);
  const [selectedCapability, setSelectedCapability] = useState<CapabilityInfo | null>(null);
  const [isCapabilityDrawerOpen, setIsCapabilityDrawerOpen] = useState(false);

  const handleToolClick = (toolId: string) => {
    const tool = allTools.find((t) => t.id === toolId);
    if (tool) {
      setSelectedTool(tool);
      setIsToolDrawerOpen(true);
    }
  };

  const handleCapabilityClick = (categoryId: string) => {
    const capability = capabilityInfo[categoryId];
    if (capability) {
      setSelectedCapability(capability);
      setIsCapabilityDrawerOpen(true);
    }
  };

  const handleToolDrawerClose = () => {
    setIsToolDrawerOpen(false);
  };

  const handleCapabilityDrawerClose = () => {
    setIsCapabilityDrawerOpen(false);
  };

  return (
    <div className="comparison-matrix">
      <div className="comparison-matrix__table-wrapper">
        <table className="comparison-matrix__table">
          <thead>
            <tr>
              <th className="comparison-matrix__header comparison-matrix__header--capability">
                Capability
              </th>
              <th className="comparison-matrix__header comparison-matrix__header--atmos">
                Atmos
              </th>
              <th className="comparison-matrix__header comparison-matrix__header--alternatives">
                Alternatives
              </th>
            </tr>
          </thead>
          <tbody>
            {toolCategories.map((category) => (
              <tr key={category.id} className="comparison-matrix__row">
                <td className="comparison-matrix__cell comparison-matrix__cell--capability">
                  <button
                    className="comparison-matrix__capability-btn"
                    onClick={() => handleCapabilityClick(category.id)}
                  >
                    {capabilityInfo[category.id]?.title || category.title}
                  </button>
                </td>
                <td className="comparison-matrix__cell comparison-matrix__cell--atmos">
                  <span className="comparison-matrix__check">
                    <RiCheckLine />
                  </span>
                </td>
                <td className="comparison-matrix__cell comparison-matrix__cell--alternatives">
                  <div className="comparison-matrix__tools">
                    {category.tools.map((tool) => (
                      <button
                        key={tool.id}
                        className="comparison-matrix__tool-badge"
                        onClick={() => handleToolClick(tool.id)}
                      >
                        {tool.name}
                      </button>
                    ))}
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <div className="comparison-matrix__legend">
        <div className="comparison-matrix__legend-item">
          <span className="comparison-matrix__check comparison-matrix__check--small">
            <RiCheckLine />
          </span>
          <span>Native capability in Atmos</span>
        </div>
      </div>

      <ToolDrawer tool={selectedTool} isOpen={isToolDrawerOpen} onClose={handleToolDrawerClose} />
      <CapabilityDrawer
        capability={selectedCapability}
        isOpen={isCapabilityDrawerOpen}
        onClose={handleCapabilityDrawerClose}
      />
    </div>
  );
};

export default ComparisonMatrix;
