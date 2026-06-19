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

export type IssueMapCanvasBlob = {
  key: string;
  path: string;
  fill: string;
  fillOpacity?: number;
  stroke?: string;
  strokeOpacity?: number;
  strokeWidth?: number;
  filter?: string;
  label?: string;
  labelX?: number;
  labelY?: number;
  labelClassName?: string;
  labelStroke?: string;
  labelStrokeOpacity?: number;
  labelStrokeWidth?: number;
  labelFill?: string;
  labelFillOpacity?: number;
  onClick?: MouseEventHandler<SVGGElement>;
  className?: string;
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
  filter?: string;
  stroke?: string;
  strokeOpacity?: number;
  strokeWidth?: number;
  strokeDasharray?: string;
  rings?: IssueMapCanvasRing[];
  label?: string;
  labelY?: number;
  labelClassName?: string;
  labelStroke?: string;
  labelStrokeOpacity?: number;
  labelStrokeWidth?: number;
  labelFill?: string;
  labelFillOpacity?: number;
  className?: string;
  dataIssue?: string;
};

// IssueMapCanvasLandmark is a permanent memory rendered onto the map. It uses a
// distinct diamond marker (vs. the round issue nodes) with a halo whose size
// reflects reinforcement, so documented decisions read as landmarks over the
// issue terrain.
export type IssueMapCanvasLandmark = {
  key: string;
  cx: number;
  cy: number;
  size: number;
  haloRadius?: number;
  fill: string;
  label?: string;
  labelClassName?: string;
  onClick?: MouseEventHandler<SVGGElement>;
  className?: string;
};

function diamondPath(cx: number, cy: number, size: number) {
  return `M ${cx} ${cy - size} L ${cx + size} ${cy} L ${cx} ${cy + size} L ${cx - size} ${cy} Z`;
}

type IssueMapCanvasProps = {
  width: number | string;
  height: number | string;
  className?: string;
  style?: CSSProperties;
  background?: ReactNode;
  gridLines?: IssueMapCanvasLine[];
  blobs?: IssueMapCanvasBlob[];
  clusters?: IssueMapCanvasCluster[];
  edges?: IssueMapCanvasLine[];
  nodes?: IssueMapCanvasNode[];
  landmarks?: IssueMapCanvasLandmark[];
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
      blobs = [],
      clusters = [],
      edges = [],
      nodes = [],
      landmarks = [],
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

      {blobs.map((blob) => (
        <g key={blob.key} onClick={blob.onClick} className={blob.className}>
          <path
            d={blob.path}
            fill={blob.fill}
            fillOpacity={blob.fillOpacity ?? 0.15}
            stroke={blob.stroke}
            strokeOpacity={blob.strokeOpacity}
            strokeWidth={blob.strokeWidth}
            filter={blob.filter}
            strokeLinejoin="round"
          />
          {blob.label && (
            <text
              x={blob.labelX}
              y={blob.labelY}
              textAnchor="middle"
              className={cn("text-[10px] font-medium", blob.labelClassName)}
              paintOrder="stroke"
              stroke={blob.labelStroke ?? "var(--background)"}
              strokeOpacity={blob.labelStrokeOpacity ?? 0.96}
              strokeWidth={blob.labelStrokeWidth ?? 6}
              fill={blob.labelFill ?? blob.fill}
              fillOpacity={blob.labelFillOpacity ?? 0.7}
            >
              {blob.label}
            </text>
          )}
        </g>
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
              y={cluster.labelY ?? cluster.cy - cluster.radius - 8}
              textAnchor="middle"
              className={cn("text-[10px] font-medium", cluster.labelClassName)}
              fill={cluster.labelFill ?? cluster.stroke ?? cluster.fill ?? "currentColor"}
              fillOpacity={cluster.labelFillOpacity ?? 0.7}
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
            filter={node.filter}
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
              paintOrder="stroke"
              stroke={node.labelStroke ?? "var(--background)"}
              strokeOpacity={node.labelStrokeOpacity ?? 0.96}
              strokeWidth={node.labelStrokeWidth ?? 5}
              fill={node.labelFill}
              fillOpacity={node.labelFillOpacity ?? 1}
            >
              {node.label}
            </text>
          )}
        </g>
      ))}

      {landmarks.map((landmark) => (
        <g
          key={landmark.key}
          onClick={landmark.onClick}
          className={landmark.className}
        >
          {landmark.haloRadius ? (
            <circle
              cx={landmark.cx}
              cy={landmark.cy}
              r={landmark.haloRadius}
              fill={landmark.fill}
              fillOpacity={0.12}
              stroke={landmark.fill}
              strokeOpacity={0.4}
            />
          ) : null}
          <path
            d={diamondPath(landmark.cx, landmark.cy, landmark.size)}
            fill={landmark.fill}
            fillOpacity={0.95}
            stroke="var(--background)"
            strokeWidth={2}
            strokeLinejoin="round"
          />
          {landmark.label && (
            <text
              x={landmark.cx}
              y={landmark.cy + landmark.size + 13}
              textAnchor="middle"
              className={cn(
                "text-[10px] font-semibold tracking-wide",
                landmark.labelClassName
              )}
              paintOrder="stroke"
              stroke="var(--background)"
              strokeOpacity={0.96}
              strokeWidth={5}
              fill={landmark.fill}
            >
              {landmark.label}
            </text>
          )}
        </g>
      ))}

      {children}
      </svg>
    );
  }
);
