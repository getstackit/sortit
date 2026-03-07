import Link from "next/link";
import { ThemeToggle } from "@/components/theme-toggle";
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarRail,
} from "@/components/ui/sidebar";

type Thing = {
  id: string;
  title: string;
};

type AppSidebarProps = React.ComponentProps<typeof Sidebar> & {
  things?: Thing[];
};

export function AppSidebar({ things = [], ...props }: AppSidebarProps) {
  return (
    <Sidebar {...props}>
      <SidebarHeader>
        <div className="flex items-center gap-2 px-2 py-1">
          <span className="text-lg font-semibold tracking-tight">bored</span>
        </div>
      </SidebarHeader>
      <SidebarContent>
        <SidebarGroup>
          <SidebarGroupLabel>Things</SidebarGroupLabel>
          <SidebarGroupContent>
            <SidebarMenu>
              {things.length === 0 && (
                <p className="px-2 py-1 text-xs text-muted-foreground/60">
                  Nothing yet
                </p>
              )}
              {things.map((thing) => (
                <SidebarMenuItem key={thing.id}>
                  <SidebarMenuButton render={<a href={`#${thing.id}`} />}>
                    <span className="truncate">{thing.title}</span>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              ))}
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
        <SidebarGroup>
          <SidebarGroupLabel>Views</SidebarGroupLabel>
          <SidebarGroupContent>
            <SidebarMenu>
              <SidebarMenuItem>
                <SidebarMenuButton render={<Link href="/" />}>
                  <span>All Issues</span>
                </SidebarMenuButton>
              </SidebarMenuItem>
              <SidebarMenuItem>
                <SidebarMenuButton render={<Link href="/map" />}>
                  <span>Map</span>
                </SidebarMenuButton>
              </SidebarMenuItem>
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
      </SidebarContent>
      <SidebarFooter>
        <div className="px-2 py-1">
          <ThemeToggle />
        </div>
      </SidebarFooter>
      <SidebarRail />
    </Sidebar>
  );
}
