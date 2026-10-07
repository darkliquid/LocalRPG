import { ContentManifestInfo } from '../types';

export async function inspectPackageFile(file: File): Promise<ContentManifestInfo> {
  try {
    if (typeof DecompressionStream !== 'undefined') {
      const stream = file.stream().pipeThrough(new DecompressionStream('gzip'));
      const reader = stream.getReader();
      let buffer = new Uint8Array(0);

      // Read first 64KB (package.yaml is first in the tar archive)
      while (buffer.length < 65536) {
        const { value, done } = await reader.read();
        if (value) {
          const next = new Uint8Array(buffer.length + value.length);
          next.set(buffer);
          next.set(value, buffer.length);
          buffer = next;
        }
        if (done) break;
      }
      reader.cancel();

      const text = new TextDecoder('utf-8', { fatal: false }).decode(buffer);
      const idMatch = text.match(/\bid:\s*([^\r\n]+)/);
      const nameMatch = text.match(/\bname:\s*([^\r\n]+)/);
      const versionMatch = text.match(/\bversion:\s*([^\r\n]+)/);
      const authorMatch = text.match(/\bauthor:\s*([^\r\n]+)/);
      const licenseMatch = text.match(/\blicense:\s*([^\r\n]+)/);
      const descMatch = text.match(/\bdescription:\s*([^\r\n]+)/);
      const hasScript = text.includes('.js') || text.includes('mechanics.js');

      return {
        id: idMatch ? idMatch[1].trim() : file.name.replace(/\.lrpgpack$/, ''),
        name: nameMatch ? nameMatch[1].trim() : file.name,
        version: versionMatch ? versionMatch[1].trim() : '1.0.0',
        author: authorMatch ? authorMatch[1].trim() : undefined,
        license: licenseMatch ? licenseMatch[1].trim() : undefined,
        description: descMatch ? descMatch[1].trim() : undefined,
        has_script: hasScript,
      };
    }
  } catch {
    // Fall back to filename
  }

  return {
    id: file.name.replace(/\.lrpgpack$/, ''),
    name: file.name,
    version: '1.0.0',
    has_script: true,
  };
}
