"use client";

import { forwardRef } from "react";
import type { CSSProperties, MouseEventHandler, ReactNode, SVGProps } from "react";
import { cn } from "@/lib/utils";

export type IssueMapCanvasLine = {
  key: string;
  x1: number;
  y1: number;
  x2: number;
  y2: number;
  stroke?: string;
  strokeOpacity?: number;
  strokeWidth?: number;
  className?: string;
};

export type IssueMapCanvasCluster = {
  key: string;
  cx: number;
  cy: number;
  radius: number;
  fill?: string;
  fillOpacity?: number;
  stroke?: string;
  strokeOpacity?: number;
  strokeWidth?: number;
  strokeDasharray?: string;
  label?: string;
  labelY?: number;
  labelClassName?: string;
  labelFill?: string;
  labelFillOpacity?: number;
};

export type IssueMapCanvasRing = {
  radius: number;
  fill?: string;
  fillOpacity?: number;
  stroke?: string;
  strokeOpacity?: number;
  strokeWidth?: number;
  strokeDasharray?: string;
};

export type IssueMapCanvasNode = {
  key: string;
  cx: number;
  cy: number;
  radius: number;
  fill: string;
  fillOpacity?: number;
  stroke?: string;
  strokeOpacity?: number;
  strokeWidth?: number;
  strokeDasharray?: string;
  rings?: IssueMapCanvasRing[];
  label?: string;
  labelY?: number;
  labelClassName?: string;
  labelFill?: string;
  labelFillOpacity?: number;
  className?: string;
  dataIssue?: string;
  onClick?: MouseEventHandler<SVGGElement>;
  onMouseEnter?: MouseEventHandler<SVGGElement>;
  onMouseLeave?: MouseEventHandler<SVGGElement>;
};

type IssueMapCanvasProps = {
  width: number;
  height: number;
  className?: string;
  style?: CSSProperties;
  background?: ReactNode;
  gridLines?: IssueMapCanvasLine[];
  clusters?: IssueMapCanvasCluster[];
  edges?: IssueMapCanvasLine[];
  nodes?: IssueMapCanvasNode[];
  children?: ReactNode;
} & Omit<SVGProps<SVGSVGElement>, "width" | "height" | "children">;

export const IssueMapCanvas = forwardRef<SVGSVGElement, IssueMapCanvasProps>(
  function IssueMapCanvas(
    {
      width,
      height,
      className,
      style,
      background,
      gridLines = [],
      clusters = [],
      edges = [],
      nodes = [],
      children,
      ...props
    },
    ref
  ) {
    return (
      <svg
        ref={ref}
        width={width}
        height={height}
        className={className}
        style={style}
        {...props}
      >
      {background}

      {gridLines.map((line) => (
        <line
          key={line.key}
          x1={line.x1}
          y1={line.y1}
          x2={line.x2}
          y2={line.y2}
          stroke={line.stroke ?? "currentColor"}
          strokeOpacity={line.strokeOpacity ?? 1}
          strokeWidth={line.strokeWidth ?? 1}
          className={line.className}
        />
      ))}

      {clusters.map((cluster) => (
        <g key={cluster.key}>
          <circle
            cx={cluster.cx}
            cy={cluster.cy}
            r={cluster.radius}
            fill={cluster.fill ?? "none"}
            fillOpacity={cluster.fillOpacity ?? 1}
            stroke={cluster.stroke}
            strokeOpacity={cluster.strokeOpacity}
            strokeWidth={cluster.strokeWidth}
            strokeDasharray={cluster.strokeDasharray}
          />
          {cluster.label && (
            <text
              x={cluster.cx}
              y={cluster.labelY ?? cluster.cy - cluster.radius - 6}
              textAnchor="middle"
              className={cn("text-[10px] font-medium", cluster.labelClassName)}
              fill={cluster.labelFill ?? cluster.stroke ?? cluster.fill ?? "currentColor"}
              fillOpacity={cluster.labelFillOpacity ?? 1}
            >
              {cluster.label}
            </text>
          )}
        </g>
      ))}

      {edges.map((edge) => (
        <line
          key={edge.key}
          x1={edge.x1}
          y1={edge.y1}
          x2={edge.x2}
          y2={edge.y2}
          stroke={edge.stroke ?? "currentColor"}
          strokeOpacity={edge.strokeOpacity ?? 1}
          strokeWidth={edge.strokeWidth ?? 1}
          className={edge.className}
        />
      ))}

      {nodes.map((node) => (
        <g
          key={node.key}
          data-issue={node.dataIssue}
          onClick={node.onClick}
          onMouseEnter={node.onMouseEnter}
          onMouseLeave={node.onMouseLeave}
          className={node.className}
        >
          {node.rings?.map((ring, index) => (
            <circle
              key={`${node.key}-ring-${index}`}
              cx={node.cx}
              cy={node.cy}
              r={ring.radius}
              fill={ring.fill ?? "none"}
              fillOpacity={ring.fillOpacity ?? 1}
              stroke={ring.stroke}
              strokeOpacity={ring.strokeOpacity}
              strokeWidth={ring.strokeWidth}
              strokeDasharray={ring.strokeDasharray}
            />
          ))}
          <circle
            cx={node.cx}
            cy={node.cy}
            r={node.radius}
            fill={node.fill}
            fillOpacity={node.fillOpacity ?? 1}
            stroke={node.stroke}
            strokeOpacity={node.strokeOpacity}
            strokeWidth={node.strokeWidth}
            strokeDasharray={node.strokeDasharray}
          />
          {node.label && (
            <text
              x={node.cx}
              y={node.labelY ?? node.cy - node.radius - 8}
              textAnchor="middle"
              className={cn("fill-foreground text-[11px] font-medium", node.labelClassName)}
              fill={node.labelFill}
              fillOpacity={node.labelFillOpacity ?? 1}
            >
              {node.label}
            </text>
          )}
        </g>
      ))}

      {children}
      </svg>
    );
  }
);
