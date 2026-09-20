import React, { useEffect, useRef } from 'react';
import { GraphData } from '../types';

interface GraphDrawerProps {
  data?: GraphData;
  onSelectNode: (nodeId: string) => void;
}

export const GraphDrawer: React.FC<GraphDrawerProps> = ({ data }) => {
  const canvasRef = useRef<HTMLCanvasElement | null>(null);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas || !data) return;
    const ctx = canvas.getContext('2d');
    if (!ctx) return;

    ctx.clearRect(0, 0, canvas.width, canvas.height);

    // Simple circular layout for nodes
    const centerX = canvas.width / 2;
    const centerY = canvas.height / 2;
    const radius = Math.min(centerX, centerY) - 40;
    const nodeCoords: Record<string, { x: number; y: number }> = {};

    data.nodes.forEach((node, i) => {
      const angle = (i / (data.nodes.length || 1)) * 2 * Math.PI;
      nodeCoords[node.id] = {
        x: centerX + radius * Math.cos(angle),
        y: centerY + radius * Math.sin(angle),
      };
    });

    // Draw links
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

    // Draw nodes
    data.nodes.forEach((node) => {
      const pos = nodeCoords[node.id];
      if (!pos) return;

      ctx.fillStyle = node.type === 'character' ? '#f59e0b' : node.type === 'npc' ? '#38bdf8' : '#a855f7';
      ctx.beginPath();
      ctx.arc(pos.x, pos.y, 8, 0, 2 * Math.PI);
      ctx.fill();

      ctx.fillStyle = '#f5f5f4';
      ctx.font = '10px Cinzel, serif';
      ctx.textAlign = 'center';
      ctx.fillText(node.label || node.id, pos.x, pos.y - 12);
    });
  }, [data]);

  return (
    <div className="space-y-4">
      <h3 className="text-lg font-cinzel text-amber-400 font-bold">Knowledge Graph</h3>
      <div className="bg-black/50 rounded-xl p-2 border border-white/5 flex justify-center shadow-inner">
        <canvas ref={canvasRef} width={340} height={340} className="rounded-lg" />
      </div>
      <div className="text-xs text-stone-400 font-mono space-y-1">
        <div>• Gold: Player Entity</div>
        <div>• Blue: Non-Player Character</div>
        <div>• Purple: Location / Lore Note</div>
      </div>
    </div>
  );
};
