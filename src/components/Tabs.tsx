import React from 'react';
import * as TabsPrimitive from '@radix-ui/react-tabs';

export interface TabItem {
  value: string;
  label: React.ReactNode;
  content?: React.ReactNode;
}

export interface TabsProps {
  value?: string;
  onValueChange?: (val: string) => void;
  defaultValue?: string;
  tabs: TabItem[];
  children?: React.ReactNode;
  className?: string;
}

export const Tabs: React.FC<TabsProps> = ({
  value,
  onValueChange,
  defaultValue,
  tabs,
  children,
  className = '',
}) => {
  return (
    <TabsPrimitive.Root
      value={value}
      onValueChange={onValueChange}
      defaultValue={defaultValue || tabs[0]?.value}
      className={`flex flex-col w-full ${className}`}
    >
      <TabsPrimitive.List className="flex items-center gap-6 border-b border-border">
        {tabs.map((tab) => (
          <TabsPrimitive.Trigger
            key={tab.value}
            value={tab.value}
            className="pb-2 text-[13px] font-medium text-text-2 hover:text-text transition-colors relative data-[state=active]:text-text after:content-[''] data-[state=active]:after:absolute data-[state=active]:after:left-0 data-[state=active]:after:right-0 data-[state=active]:after:-bottom-[1px] data-[state=active]:after:h-[2px] data-[state=active]:after:bg-text"
          >
            {tab.label}
          </TabsPrimitive.Trigger>
        ))}
      </TabsPrimitive.List>

      {tabs.map((tab) =>
        tab.content ? (
          <TabsPrimitive.Content
            key={tab.value}
            value={tab.value}
            className="pt-4 focus:outline-none"
          >
            {tab.content}
          </TabsPrimitive.Content>
        ) : null
      )}
      {children}
    </TabsPrimitive.Root>
  );
};

