"use client";

import { useMemo } from "react";
import { ChevronDownIcon, FilterIcon, XIcon } from "lucide-react";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import {
  SidebarContent,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
} from "@/components/ui/sidebar";
import { entityStyle } from "@/lib/entity-colors";
import type { IssueRecord } from "@/lib/issues";


export type IssuesFilter = {
  tags: string[];
  assignees: string[];
  includeClosed: boolean;
};

export const EMPTY_FILTER: IssuesFilter = { tags: [], assignees: [], includeClosed: false };

export function isFilterActive(filter: IssuesFilter): boolean {
  return filter.tags.length > 0 || filter.assignees.length > 0;
}

export function applyFilter(
  issues: IssueRecord[],
  filter: IssuesFilter
): IssueRecord[] {
  return issues.filter((issue) => {
    if (
      filter.tags.length > 0 &&
      !filter.tags.every((t) => issue.tags.includes(t))
    ) {
      return false;
    }
    if (
      filter.assignees.length > 0 &&
      !filter.assignees.includes(issue.assignedTo ?? "")
    ) {
      return false;
    }
    return true;
  });
}

function CheckItem({
  label,
  checked,
  onChange,
  style,
}: {
  label: string;
  checked: boolean;
  onChange: (checked: boolean) => void;
  style?: React.CSSProperties;
}) {
  return (
    <label className="flex cursor-pointer items-center gap-2 rounded-lg px-2 py-1.5 text-sm transition-colors hover:bg-sidebar-accent/80">
      <Checkbox
        checked={checked}
        onCheckedChange={onChange}
      />
      <span className="truncate text-xs" style={style}>
        {label}
      </span>
    </label>
  );
}

function FilterSection({
  title,
  children,
  defaultOpen = true,
}: {
  title: string;
  children: React.ReactNode;
  defaultOpen?: boolean;
}) {
  return (
    <Collapsible defaultOpen={defaultOpen}>
      <SidebarGroup className="space-y-1">
        <CollapsibleTrigger className="flex w-full items-center justify-between px-2">
          <SidebarGroupLabel className="p-0 text-[11px] font-medium uppercase tracking-[0.16em] text-muted-foreground/60">
            {title}
          </SidebarGroupLabel>
          <ChevronDownIcon className="size-3.5 text-muted-foreground/50 transition-transform [[data-open]_&]:rotate-180" />
        </CollapsibleTrigger>
        <CollapsibleContent>
          <SidebarGroupContent>{children}</SidebarGroupContent>
        </CollapsibleContent>
      </SidebarGroup>
    </Collapsible>
  );
}

type IssuesFilterSidebarProps = {
  issues: IssueRecord[];
  filter: IssuesFilter;
  onFilterChange: (filter: IssuesFilter) => void;
};

export function IssuesFilterSidebar({
  issues,
  filter,
  onFilterChange,
}: IssuesFilterSidebarProps) {
  const { allTags, allAssignees } = useMemo(() => {
    const tagCounts = new Map<string, number>();
    const assigneeCounts = new Map<string, number>();

    for (const issue of issues) {
      for (const tag of issue.tags) {
        tagCounts.set(tag, (tagCounts.get(tag) ?? 0) + 1);
      }
      if (issue.assignedTo) {
        assigneeCounts.set(
          issue.assignedTo,
          (assigneeCounts.get(issue.assignedTo) ?? 0) + 1
        );
      }
    }

    return {
      allTags: [...tagCounts.entries()]
        .sort((a, b) => b[1] - a[1])
        .map(([name, count]) => ({ name, count })),
      allAssignees: [...assigneeCounts.entries()]
        .sort((a, b) => b[1] - a[1])
        .map(([name, count]) => ({ name, count })),
    };
  }, [issues]);

  const active = isFilterActive(filter);

  function toggleTag(tag: string, checked: boolean) {
    onFilterChange({
      ...filter,
      tags: checked
        ? [...filter.tags, tag]
        : filter.tags.filter((t) => t !== tag),
    });
  }

  function toggleAssignee(assignee: string, checked: boolean) {
    onFilterChange({
      ...filter,
      assignees: checked
        ? [...filter.assignees, assignee]
        : filter.assignees.filter((a) => a !== assignee),
    });
  }

  return (
    <div className="flex h-full flex-col border-l border-border/40">
      <SidebarHeader className="flex-row items-center justify-between p-3">
        <div className="flex items-center gap-2 text-xs font-medium text-muted-foreground">
          <FilterIcon className="size-3.5" />
          Filters
        </div>
        {active && (
          <button
            type="button"
            onClick={() => onFilterChange(EMPTY_FILTER)}
            className="flex items-center gap-1 rounded-md px-1.5 py-0.5 text-[11px] text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
          >
            <XIcon className="size-3" />
            Clear
          </button>
        )}
      </SidebarHeader>

      <SidebarContent className="px-1">
        <FilterSection title="Status">
          <CheckItem
            label="Include closed"
            checked={filter.includeClosed}
            onChange={(checked) =>
              onFilterChange({ ...filter, includeClosed: checked })
            }
          />
        </FilterSection>

        {allTags.length > 0 && (
          <FilterSection title="Tags">
            {allTags.map(({ name, count }) => (
              <CheckItem
                key={name}
                label={`${name} (${count})`}
                checked={filter.tags.includes(name)}
                onChange={(checked) => toggleTag(name, checked)}
                style={entityStyle(name)}
              />
            ))}
          </FilterSection>
        )}

        {allAssignees.length > 0 && (
          <FilterSection title="Assignee">
            {allAssignees.map(({ name, count }) => (
              <CheckItem
                key={name}
                label={`${name} (${count})`}
                checked={filter.assignees.includes(name)}
                onChange={(checked) => toggleAssignee(name, checked)}
              />
            ))}
          </FilterSection>
        )}

        {allTags.length === 0 && allAssignees.length === 0 && (
          <div className="px-3 py-6 text-center text-xs text-muted-foreground/50">
            No filter options available
          </div>
        )}
      </SidebarContent>
    </div>
  );
}
