import { render, screen, fireEvent } from '@testing-library/react';
import { describe, it, expect, vi } from 'vitest';
import { ContextLimitsPanel } from './ContextLimitsPanel';
import type { AppConfig } from '../../types';

const mockConfig: AppConfig = {
  version: '1',
  paths: { systems: '', worlds: '', games: '', cache: '' },
  agents: {
    default_role: 'gm',
    roles: {},
    context_token_budget: 4500,
    recent_turn_window: 6,
    recent_turn_char_limit: 1200,
    scene_recall_turns: 4,
    turn_timeout_seconds: 45,
    chunk_timeout_seconds: 15,
    tool_rounds: 3,
  },
  media: {
    tts: { type: 'disabled', auto_play: false, master_volume: 1 },
    stt: { type: 'disabled' },
    image: { type: 'disabled', auto_generate: false },
  },
  preferences: {
    streaming: false,
    typing_speed_ms: 0,
    cinematic_effects: false,
    font_scale: 'medium',
  },
};

describe('ContextLimitsPanel', () => {
  it('renders limit inputs with initial values from config', () => {
    render(<ContextLimitsPanel config={mockConfig} setConfig={vi.fn()} advanced={true} />);

    expect(screen.getByText('Context & Response Limits')).toBeInTheDocument();
    expect(screen.getByDisplayValue('4500')).toBeInTheDocument();
    expect(screen.getByDisplayValue('6')).toBeInTheDocument();
    expect(screen.getByDisplayValue('1200')).toBeInTheDocument();
    expect(screen.getByDisplayValue('45')).toBeInTheDocument();
  });

  it('calls setConfig when Context Budget is updated', () => {
    const setConfig = vi.fn();
    render(<ContextLimitsPanel config={mockConfig} setConfig={setConfig} advanced={true} />);

    const budgetInput = screen.getByDisplayValue('4500');
    fireEvent.change(budgetInput, { target: { value: '8000' } });

    expect(setConfig).toHaveBeenCalledWith({
      ...mockConfig,
      agents: {
        ...mockConfig.agents,
        context_token_budget: 8000,
      },
    });
  });

  it('calls setConfig when Remembered Turns is updated', () => {
    const setConfig = vi.fn();
    render(<ContextLimitsPanel config={mockConfig} setConfig={setConfig} advanced={true} />);

    const turnsInput = screen.getByDisplayValue('6');
    fireEvent.change(turnsInput, { target: { value: '10' } });

    expect(setConfig).toHaveBeenCalledWith({
      ...mockConfig,
      agents: {
        ...mockConfig.agents,
        recent_turn_window: 10,
      },
    });
  });

  it('handles invalid numeric input by falling back to safe defaults', () => {
    const setConfig = vi.fn();
    render(<ContextLimitsPanel config={mockConfig} setConfig={setConfig} advanced={true} />);

    const budgetInput = screen.getByDisplayValue('4500');
    fireEvent.change(budgetInput, { target: { value: 'not-a-number' } });

    expect(setConfig).toHaveBeenCalledWith({
      ...mockConfig,
      agents: {
        ...mockConfig.agents,
        context_token_budget: 0,
      },
    });
  });
});
