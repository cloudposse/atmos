import React, { useState, useEffect } from 'react';
import { motion, AnimatePresence } from 'framer-motion';
import { RiArrowDownSLine } from 'react-icons/ri';
import * as Icons from 'react-icons/ri';
import ToolCard from '../ToolCard';
import ToolDrawer, { type Tool } from '../ToolDrawer';
import './index.css';

export interface ToolCategoryProps {
  id: string;
  icon: string;
  title: string;
  tagline: string;
  tools: Tool[];
  defaultExpanded?: boolean;
  expandAll?: boolean;
  index?: number;
}

const ToolCategory: React.FC<ToolCategoryProps> = ({
  id,
  icon,
  title,
  tagline,
  tools,
  defaultExpanded = false,
  expandAll = false,
  index = 0,
}) => {
  const [isExpanded, setIsExpanded] = useState(defaultExpanded);
  const [selectedTool, setSelectedTool] = useState<Tool | null>(null);
  const [isDrawerOpen, setIsDrawerOpen] = useState(false);

  // Sync with expandAll prop.
  useEffect(() => {
    setIsExpanded(expandAll);
  }, [expandAll]);

  // Dynamically get the icon component.
  const IconComponent = (Icons as Record<string, React.ComponentType<{ className?: string }>>)[
    icon
  ] || Icons.RiQuestionLine;

  const handleToolClick = (tool: Tool) => {
    setSelectedTool(tool);
    setIsDrawerOpen(true);
  };

  const handleDrawerClose = () => {
    setIsDrawerOpen(false);
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      setIsExpanded(!isExpanded);
    }
  };

  return (
    <motion.div
      id={id}
      className="tool-category"
      initial={{ opacity: 0, y: 20 }}
      whileInView={{ opacity: 1, y: 0 }}
      viewport={{ once: true, margin: '-30px' }}
      transition={{ duration: 0.4, delay: index * 0.05 }}
    >
      <div
        className={`tool-category__header ${isExpanded ? 'tool-category__header--expanded' : ''}`}
        onClick={() => setIsExpanded(!isExpanded)}
        onKeyDown={handleKeyDown}
        role="button"
        tabIndex={0}
        aria-expanded={isExpanded}
      >
        <div className="tool-category__icon">
          <IconComponent />
        </div>
        <div className="tool-category__info">
          <h3 className="tool-category__title">{title}</h3>
          <p className="tool-category__tagline">{tagline}</p>
        </div>
        <div className="tool-category__meta">
          <span className="tool-category__count">{tools.length} tools</span>
          <motion.div
            className="tool-category__expand"
            animate={{ rotate: isExpanded ? 180 : 0 }}
            transition={{ duration: 0.2 }}
          >
            <RiArrowDownSLine />
          </motion.div>
        </div>
      </div>

      <AnimatePresence>
        {isExpanded && (
          <motion.div
            className="tool-category__content"
            initial={{ height: 0, opacity: 0 }}
            animate={{ height: 'auto', opacity: 1 }}
            exit={{ height: 0, opacity: 0 }}
            transition={{ duration: 0.3, ease: 'easeInOut' }}
          >
            <div className="tool-category__grid">
              {tools.map((tool, toolIndex) => (
                <ToolCard
                  key={tool.id}
                  tool={tool}
                  onClick={handleToolClick}
                  index={toolIndex}
                />
              ))}
            </div>
          </motion.div>
        )}
      </AnimatePresence>

      <ToolDrawer
        tool={selectedTool}
        isOpen={isDrawerOpen}
        onClose={handleDrawerClose}
      />
    </motion.div>
  );
};

export default ToolCategory;
