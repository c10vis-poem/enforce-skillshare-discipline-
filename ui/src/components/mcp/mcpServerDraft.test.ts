import { describe, expect, it } from 'vitest';
import { initialServerDraft, serveSkillsCommand, serveSkillsTarget } from './mcpServerDraft';

describe('MCP server drafts', () => {
  it('starts from the Pi settings of a server being edited', () => {
    expect(JSON.parse(initialServerDraft({ command: 'docs', piOptions: { exposure: 'direct' } }, 'docs', ['pi'], false).piOptions)).toEqual({ exposure: 'direct' });
  });

  it('starts from the tool policy of a server being edited', () => {
    expect(initialServerDraft({ command: 'docs', tools: { deny: ['delete_*'] } }, 'docs', ['pi'], false).tools).toEqual({ deny: ['delete_*'] });
  });

  it('keeps only Agents with a switch in the targets a new off switch starts with', () => {
    expect(initialServerDraft(undefined, '', ['claude', 'cursor', 'pi'], true).targets).toEqual(['claude', 'pi']);
  });

  it('reads back the skills target of a command it builds, and nothing from other commands', () => {
    expect(serveSkillsTarget(serveSkillsCommand('claude', true).split(' '))).toBe('claude');
    expect(serveSkillsTarget(serveSkillsCommand('', false).split(' '))).toBe('');
    expect(serveSkillsTarget(['skillshare', 'mcp', 'serve', '--http', ':8765'])).toBeUndefined();
    expect(serveSkillsTarget(['npx', 'skillshare', 'mcp', 'serve'])).toBeUndefined();
  });
});
