import { render, screen, fireEvent } from '@testing-library/react';
import { describe, it, expect, vi } from 'vitest';
import { AgentsPanel } from './AgentsPanel';
import type { AppConfig, AgentRoleConfig, ProviderDescriptor } from '../../types';

const mockConfig: AppConfig = {
  version: '1',
  paths: { systems: '', worlds: '', games: '', cache: '' },
  agents: {
    default_role: 'gm',
    action_echo: true,
    roles: {
      gm: { type: 'gemini', model: 'gemini-2.5-flash' },
      extractor: { type: 'inherit', inherit_from: 'gm' },
    },
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

const mockPresets: Record<string, { label: string; description: string; config: AgentRoleConfig; tier?: string }> = {
  'gemini-flash': {
    label: 'Gemini Flash',
    description: 'Fast cloud LLM',
    config: { type: 'gemini', model: 'gemini-2.5-flash' },
    tier: 'cloud',
  },
};

const mockDescriptors: ProviderDescriptor[] = [
  {
    id: 'llm:gemini',
    family: 'llm',
    label: 'Google Gemini',
    description: 'Cloud LLM via Gemini API',
    source: 'cloud',
    features: ['key_required'],
  },
];

describe('AgentsPanel', () => {
  it('renders roles tab by default with action echo checkbox and role selector', () => {
    const setConfig = vi.fn();
    render(
      <AgentsPanel
        config={mockConfig}
        setConfig={setConfig}
        advanced={false}
        agentPresets={mockPresets}
        llmDescriptors={mockDescriptors}
        tierCaveats={{}}
        handleTestProvider={vi.fn()}
        testingCategory={null}
        testResult={null}
      />,
    );

    expect(screen.getByText("Restate the player's action")).toBeInTheDocument();
    expect(screen.getByText('AI Agents & Role Routing')).toBeInTheDocument();

    // Toggle action echo checkbox
    const echoCheckbox = screen.getByRole('checkbox');
    expect(echoCheckbox).toBeChecked();
    fireEvent.click(echoCheckbox);
    expect(setConfig).toHaveBeenCalledWith({
      ...mockConfig,
      agents: { ...mockConfig.agents, action_echo: false },
    });
  });

  it('loads preset when selected from preset dropdown', () => {
    const setConfig = vi.fn();
    render(
      <AgentsPanel
        config={mockConfig}
        setConfig={setConfig}
        advanced={false}
        agentPresets={mockPresets}
        llmDescriptors={mockDescriptors}
        tierCaveats={{}}
        handleTestProvider={vi.fn()}
        testingCategory={null}
        testResult={null}
      />,
    );

    const presetSelect = screen.getByDisplayValue('⚡ Load Preset...');
    fireEvent.change(presetSelect, { target: { value: 'gemini-flash' } });

    expect(setConfig).toHaveBeenCalledWith({
      ...mockConfig,
      agents: {
        ...mockConfig.agents,
        roles: {
          ...mockConfig.agents.roles,
          gm: { type: 'gemini', model: 'gemini-2.5-flash' },
        },
      },
    });
  });

  it('switches to Routing tab and displays role rows', () => {
    render(
      <AgentsPanel
        config={mockConfig}
        setConfig={vi.fn()}
        advanced={false}
        agentPresets={mockPresets}
        llmDescriptors={mockDescriptors}
        tierCaveats={{}}
        handleTestProvider={vi.fn()}
        testingCategory={null}
        testResult={null}
      />,
    );

    fireEvent.click(screen.getByRole('tab', { name: 'Routing' }));

    expect(screen.getByRole('heading', { name: 'Role Routing' })).toBeInTheDocument();
    expect(
      screen.getByText('Every role resolves to one adapter. Select a row to edit that role on the Roles tab.'),
    ).toBeInTheDocument();
  });

  it('shows Advanced tab when advanced is true and renders limits panels', () => {
    render(
      <AgentsPanel
        config={mockConfig}
        setConfig={vi.fn()}
        advanced={true}
        agentPresets={mockPresets}
        llmDescriptors={mockDescriptors}
        tierCaveats={{}}
        handleTestProvider={vi.fn()}
        testingCategory={null}
        testResult={null}
      />,
    );

    const advancedTab = screen.getByRole('tab', { name: 'Advanced' });
    expect(advancedTab).toBeInTheDocument();

    fireEvent.click(advancedTab);

    expect(screen.getByText('Context & Response Limits')).toBeInTheDocument();
    expect(screen.getByText('Generation Limits')).toBeInTheDocument();
  });
});
