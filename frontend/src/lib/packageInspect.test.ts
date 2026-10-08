import { describe, it, expect } from 'vitest';
import { inspectPackageFile } from './packageInspect';

describe('inspectPackageFile', () => {
  it('infers type and strips extension for .lrpgworld file fallback', async () => {
    const file = new File(['not a gzip stream'], 'my_epic_world-1.0.0.lrpgworld');
    const manifest = await inspectPackageFile(file);

    expect(manifest.id).toBe('my_epic_world-1.0.0');
    expect(manifest.name).toBe('my_epic_world-1.0.0.lrpgworld');
    expect(manifest.type).toBe('world');
  });

  it('infers type and strips extension for .lrpgsystem file fallback', async () => {
    const file = new File(['not a gzip stream'], 'd20_rules-0.1.0.lrpgsystem');
    const manifest = await inspectPackageFile(file);

    expect(manifest.id).toBe('d20_rules-0.1.0');
    expect(manifest.type).toBe('system');
  });

  it('handles .lrpgpack without inferring specific type', async () => {
    const file = new File(['not a gzip stream'], 'generic_pack-1.0.0.lrpgpack');
    const manifest = await inspectPackageFile(file);

    expect(manifest.id).toBe('generic_pack-1.0.0');
    expect(manifest.type).toBeUndefined();
  });
});
