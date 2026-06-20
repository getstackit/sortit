import {
  Activity,
  BarChart3,
  BookMarked,
  Bug,
  CircleDot,
  Layers,
  Map,
  Settings,
  Tags,
  Users,
} from "lucide-react";
import type { LucideIcon } from "lucide-react";

export type NavGroup = "work" | "explore" | "system";

export type NavItem = {
  /** Display label used in the sidebar and command palette. */
  label: string;
  /** Route. Never changed by the IA refactor. */
  href: string;
  icon: LucideIcon;
  group: NavGroup;
  /** Single key for the `G <key>` go-to sequence. */
  shortcut?: string;
  /** Hidden from the primary sidebar outside dev (still reachable via palette + URL). */
  devOnly?: boolean;
};

export const NAV_GROUPS: { id: NavGroup; label: string }[] = [
  { id: "work", label: "Work" },
  { id: "explore", label: "Explore" },
  { id: "system", label: "System" },
];

export const NAV_ITEMS: NavItem[] = [
  { label: "Issues", href: "/", icon: CircleDot, group: "work", shortcut: "i" },
  { label: "Activity", href: "/activity", icon: Activity, group: "work", shortcut: "a" },
  { label: "Memories", href: "/memories", icon: BookMarked, group: "work" },
  { label: "Issue Map", href: "/map", icon: Map, group: "explore", shortcut: "m" },
  { label: "Tag Map", href: "/tags", icon: Tags, group: "explore", shortcut: "t" },
  { label: "Regions", href: "/regions", icon: Layers, group: "explore", shortcut: "r" },
  { label: "People", href: "/people", icon: Users, group: "explore", shortcut: "p" },
  { label: "Analytics", href: "/analytics", icon: BarChart3, group: "explore", shortcut: "y" },
  { label: "Settings", href: "/settings", icon: Settings, group: "system", shortcut: "s" },
  { label: "Debug", href: "/debug", icon: Bug, group: "system", shortcut: "d", devOnly: true },
];

/**
 * Whether dev-only nav items appear in the primary sidebar. The `/debug` route
 * itself is never blocked, and the command palette always lists every item, so
 * no view is ever truly unreachable.
 */
export const SHOW_DEV_NAV =
  process.env.NODE_ENV !== "production" ||
  process.env.NEXT_PUBLIC_SHOW_DEBUG_NAV === "true";

export function navItemsForGroup(group: NavGroup, includeDevOnly: boolean): NavItem[] {
  return NAV_ITEMS.filter(
    (item) => item.group === group && (includeDevOnly || !item.devOnly)
  );
}
