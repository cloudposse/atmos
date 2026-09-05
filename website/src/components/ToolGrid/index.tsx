import React, { useState } from 'react';
import ToolCard from '../ToolCard';
import ToolDrawer, { type Tool } from '../ToolDrawer';
import './index.css';

interface ToolGridProps {
  tools: Tool[];
  columns?: 1 | 2 | 3;
}

const ToolGrid: React.FC<ToolGridProps> = ({ tools, columns = 2 }) => {
  const [selectedTool, setSelectedTool] = useState<Tool | null>(null);
  const [isDrawerOpen, setIsDrawerOpen] = useState(false);

  const handleToolClick = (tool: Tool) => {
    setSelectedTool(tool);
    setIsDrawerOpen(true);
  };

  const handleDrawerClose = () => {
    setIsDrawerOpen(false);
  };

  return (
    <>
      <div className={`tool-grid tool-grid--cols-${columns}`}>
        {tools.map((tool, index) => (
          <ToolCard
            key={tool.id}
            tool={tool}
            onClick={handleToolClick}
            index={index}
          />
        ))}
      </div>
      <ToolDrawer
        tool={selectedTool}
        isOpen={isDrawerOpen}
        onClose={handleDrawerClose}
      />
    </>
  );
};

export default ToolGrid;
export type { Tool };
