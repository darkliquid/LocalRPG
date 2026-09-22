import React, { useEffect, useRef } from 'react';
import { GraphData } from '../types';
import { Network } from 'lucide-react';

interface GraphDrawerProps {
  data?: GraphData;
  onSelectNode: (nodeId: string) => void;
}

const CANVAS_SIZE = 360;
const NODE_RADIUS = 9;
// A click within this many pixels of a node centre selects it, which is a larger
// target than the dot itself so the graph is usable without pixel precision.
const HIT_RADIUS = 16;

export const GraphDrawer: React.FC<GraphDrawerProps> = ({ data, onSelectNode }) => {
  const canvasRef = useRef<HTMLCanvasElement | null>(null);
  // Hit testing needs the layout that was last drawn, not a recomputation.
  const coordsRef = useRef<Record<string, { x: number; y: number }>>({});

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas || !data) return;
    const ctx = canvas.getContext('2d');
    if (!ctx) return;

    ctx.clearRect(0, 0, canvas.width, canvas.height);

    const nodes = data.nodes;
    const centerX = canvas.width / 2;
    const centerY = canvas.height / 2;
    const radius = Math.min(centerX, centerY) - 46;
    const nodeCoords: Record<string, { x: number; y: number }> = {};

    nodes.forEach((node, i) => {
      const angle = (i / (nodes.length || 1)) * 2 * Math.PI - Math.PI / 2;
      nodeCoords[node.id] = {
        x: centerX + radius * Math.cos(angle),
        y: centerY + radius * Math.sin(angle),
      };
    });
    coordsRef.current = nodeCoords;

    // Draw links first so nodes sit on top of their own lines.
    ctx.strokeStyle = 'rgba(168, 162, 158, 0.3)';
    ctx.lineWidth = 1.5;
    data.links.forEach((link) => {
      const src = nodeCoords[link.source];
      const dst = nodeCoords[link.target];
      if (src && dst) {
        ctx.beginPath();
        ctx.moveTo(src.x, src.y);
        ctx.lineTo(dst.x, dst.y);
        ctx.stroke();
      }
    });

    // Labels crowd once the corpus grows, so their size follows the node count.
    const fontSize = nodes.length > 24 ? 8 : 10;

    nodes.forEach((node) => {
      const pos = nodeCoords[node.id];
      if (!pos) return;

      ctx.fillStyle =
        node.type === 'character' ? '#f59e0b' :
        node.type === 'npc' ? '#38bdf8' :
        node.type === 'location' ? '#a855f7' : '#78716c';
      ctx.beginPath();
      ctx.arc(pos.x, pos.y, NODE_RADIUS, 0, 2 * Math.PI);
      ctx.fill();

      const label = node.label || node.id;
      ctx.fillStyle = '#f5f5f4';
      ctx.font = `${fontSize}px Cinzel, serif`;
      ctx.textAlign = 'center';
      ctx.fillText(label.length > 18 ? `${label.slice(0, 17)}…` : label, pos.x, pos.y - NODE_RADIUS - 4);
    });
  }, [data]);

  const handleClick = (event: React.MouseEvent<HTMLCanvasElement>) => {
    const canvas = canvasRef.current;
    if (!canvas) return;

    const rect = canvas.getBoundingClientRect();
    const x = (event.clientX - rect.left) * (canvas.width / rect.width);
    const y = (event.clientY - rect.top) * (canvas.height / rect.height);

    let closest: string | null = null;
    let closestDistance = Infinity;
    for (const [id, pos] of Object.entries(coordsRef.current)) {
      const distance = Math.hypot(pos.x - x, pos.y - y);
      if (distance <= HIT_RADIUS && distance < closestDistance) {
        closest = id;
        closestDistance = distance;
      }
    }
    if (closest) onSelectNode(closest);
  };

  const isEmpty = !data || data.nodes.length === 0;

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h3 className="text-lg font-cinzel text-amber-400 font-bold flex items-center gap-1.5">
          <Network className="w-4 h-4" />
          <span>Knowledge Graph</span>
        </h3>
        {!isEmpty && <span className="text-xs font-mono text-stone-400">{data.nodes.length} nodes · {data.links.length} links</span>}
      </div>

      {isEmpty ? (
        <p className="text-stone-500 text-xs italic bg-black/30 p-3 rounded-xl border border-white/5">
          No entities yet. Play a turn and the world will grow.
        </p>
      ) : (
        <>
          <div className="bg-black/50 rounded-xl p-2 border border-white/5 flex justify-center shadow-inner">
            <canvas
              ref={canvasRef}
              width={CANVAS_SIZE}
              height={CANVAS_SIZE}
              onClick={handleClick}
              className="rounded-lg cursor-pointer"
            />
          </div>
          <div className="text-xs text-stone-400 font-mono space-y-1">
            <div>• Gold: Character</div>
            <div>• Blue: NPC</div>
            <div>• Purple: Location</div>
            <div>• Grey: Lore, arcs and other notes</div>
            <div className="text-amber-400/80">Click a node to open its note.</div>
          </div>
        </>
      )}
    </div>
  );
};
