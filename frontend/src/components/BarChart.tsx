import { useEffect, useRef } from 'react';
import * as d3 from 'd3';

interface Stat {
  label: string;
  value: number;
}

interface BarChartProps {
  data: Stat[];
  width?: number;
  height?: number;
  color?: string;
}

export function BarChart({ data, width = 400, height = 250, color = "#2563eb" }: BarChartProps) {
  const svgRef = useRef<SVGSVGElement>(null);

  useEffect(() => {
    if (!svgRef.current) return;

    const svg = d3.select(svgRef.current);
    svg.selectAll("*").remove();

    if (!data || data.length === 0) return;

    const margin = { top: 20, right: 30, bottom: 40, left: 40 };
    const chartWidth = width - margin.left - margin.right;
    const chartHeight = height - margin.top - margin.bottom;

    const x = d3.scaleBand()
      .domain(data.map(d => d.label))
      .range([0, chartWidth])
      .padding(0.1);

    const y = d3.scaleLinear()
      .domain([0, d3.max(data, d => d.value) || 0])
      .nice()
      .range([chartHeight, 0]);

    const g = svg.append("g")
      .attr("transform", `translate(${margin.left},${margin.top})`);

    // X axis
    g.append("g")
      .attr("transform", `translate(0,${chartHeight})`)
      .call(d3.axisBottom(x))
      .selectAll("text")
      .attr("transform", "translate(-10,0)rotate(-45)")
      .style("text-anchor", "end");

    // Y axis
    g.append("g")
      .call(d3.axisLeft(y).ticks(5));

    // Bars
    g.selectAll(".bar")
      .data(data)
      .enter().append("rect")
      .attr("class", "bar")
      .attr("x", d => x(d.label) || 0)
      .attr("y", d => y(d.value))
      .attr("width", x.bandwidth())
      .attr("height", d => chartHeight - y(d.value))
      .attr("fill", color)
      .attr("rx", 4)
      .append("title")
      .text((d) => `${d.label}: ${d.value}`);

  }, [data, width, height, color]);

  if (!data || data.length === 0) {
    return <p className="empty-state">No data for the current filter.</p>;
  }

  return (
    <div style={{ width: '100%', overflow: 'hidden' }}>
      <svg ref={svgRef} width="100%" height={height} viewBox={`0 0 ${width} ${height}`} preserveAspectRatio="xMidYMid meet"></svg>
    </div>
  );
}
