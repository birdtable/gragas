// The tabs part: Base UI's tabs, which carry the roles, the arrow-key roving and
// the panel wiring. The list is named by its label.
import { Tabs as BaseTabs } from "@base-ui/react/tabs";
import type { ReactElement, ReactNode } from "react";
import "../base.css";
import "./tabs.css";

export const states = ["first-selected", "second-selected"] as const;

export type Tab = Readonly<{ id: string; label: string; panel: ReactNode }>;

export type TabsProps = Readonly<{
  label: string;
  tabs: readonly Tab[];
  selected?: string;
}>;

export function Tabs({ label, tabs, selected }: TabsProps): ReactElement {
  const first = tabs[0]?.id;
  const initial = selected ?? first;
  return (
    <div data-part="tabs">
      <BaseTabs.Root
        {...(initial === undefined ? {} : { defaultValue: initial })}
      >
        <BaseTabs.List aria-label={label} className="gr-tabs-list">
          {tabs.map((tab) => (
            <BaseTabs.Tab
              key={tab.id}
              value={tab.id}
              className="gr-tab gr-target"
            >
              {tab.label}
            </BaseTabs.Tab>
          ))}
        </BaseTabs.List>
        {tabs.map((tab) => (
          <BaseTabs.Panel key={tab.id} value={tab.id} className="gr-tab-panel">
            {tab.panel}
          </BaseTabs.Panel>
        ))}
      </BaseTabs.Root>
    </div>
  );
}
