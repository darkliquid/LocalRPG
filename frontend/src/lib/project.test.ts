import { describe, expect, it } from 'vitest';
import { PROJECT_ISSUES_URL, PROJECT_REPO_URL } from './project';

describe('project links', () => {
  it('derives the issues URL from the repository URL', () => {
    expect(PROJECT_ISSUES_URL).toBe(`${PROJECT_REPO_URL}/issues`);
  });
});
