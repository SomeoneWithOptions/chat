import { cleanup, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import type { EnhanceResponse } from '../lib/api';
import * as api from '../lib/api';
import PromptEnhanceModal from './PromptEnhanceModal';

const mockEnhancePrompt = vi.fn();

beforeEach(() => {
  mockEnhancePrompt.mockReset();
  vi.spyOn(api, 'enhancePrompt').mockImplementation(mockEnhancePrompt);
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const defaultProps = {
  isOpen: true,
  onClose: vi.fn(),
  prompt: 'Write a function that sorts an array',
  modelId: 'openai/gpt-4',
  onUsePrompt: vi.fn(),
};

const sampleQuestions: EnhanceResponse = {
  questions: [
    {
      id: 'q1',
      text: 'What programming language?',
      type: 'single_select' as const,
      options: [
        { id: 'opt1', label: 'JavaScript' },
        { id: 'opt2', label: 'Python' },
        { id: 'opt3', label: 'TypeScript' },
      ],
    },
    {
      id: 'q2',
      text: 'What sorting algorithm?',
      type: 'multi_select' as const,
      options: [
        { id: 'opt4', label: 'QuickSort' },
        { id: 'opt5', label: 'MergeSort' },
      ],
    },
    {
      id: 'q3',
      text: 'Should the function handle edge cases?',
      type: 'yes_no' as const,
      options: [
        { id: 'yes', label: 'Yes' },
        { id: 'no', label: 'No' },
      ],
    },
  ],
};

const sampleEnhanced: EnhanceResponse = {
  enhancedPrompt: 'Write a TypeScript function that sorts an array of numbers using QuickSort, handling edge cases like empty arrays and duplicate values.',
};

describe('PromptEnhanceModal', () => {
  it('renders loading state on open', () => {
    mockEnhancePrompt.mockReturnValue(new Promise(() => {})); // never resolves
    render(<PromptEnhanceModal {...defaultProps} />);

    expect(screen.getByText('Analyzing your prompt...')).toBeInTheDocument();
  });

  it('renders questions after API response', async () => {
    mockEnhancePrompt.mockResolvedValueOnce(sampleQuestions);
    render(<PromptEnhanceModal {...defaultProps} />);

    await waitFor(() => {
      expect(screen.getByText('What programming language?')).toBeInTheDocument();
    });

    expect(screen.getByText('JavaScript')).toBeInTheDocument();
    expect(screen.getByText('Python')).toBeInTheDocument();
    expect(screen.getByText('TypeScript')).toBeInTheDocument();
  });

  it('single select allows only one option', async () => {
    mockEnhancePrompt.mockResolvedValueOnce(sampleQuestions);
    const user = userEvent.setup();
    render(<PromptEnhanceModal {...defaultProps} />);

    await waitFor(() => {
      expect(screen.getByText('JavaScript')).toBeInTheDocument();
    });

    await user.click(screen.getByText('JavaScript'));
    expect(screen.getByText('JavaScript')).toHaveClass('selected');

    await user.click(screen.getByText('Python'));
    expect(screen.getByText('Python')).toHaveClass('selected');
    expect(screen.getByText('JavaScript')).not.toHaveClass('selected');
  });

  it('multi select allows multiple options', async () => {
    mockEnhancePrompt.mockResolvedValueOnce(sampleQuestions);
    const user = userEvent.setup();
    render(<PromptEnhanceModal {...defaultProps} />);

    await waitFor(() => {
      expect(screen.getByText('QuickSort')).toBeInTheDocument();
    });

    await user.click(screen.getByText('QuickSort'));
    await user.click(screen.getByText('MergeSort'));

    expect(screen.getByText('QuickSort')).toHaveClass('selected');
    expect(screen.getByText('MergeSort')).toHaveClass('selected');
  });

  it('yes/no renders two options', async () => {
    mockEnhancePrompt.mockResolvedValueOnce(sampleQuestions);
    render(<PromptEnhanceModal {...defaultProps} />);

    await waitFor(() => {
      expect(screen.getByText('Should the function handle edge cases?')).toBeInTheDocument();
    });

    const yesButtons = screen.getAllByText('Yes');
    const noButtons = screen.getAllByText('No');
    expect(yesButtons.length).toBeGreaterThanOrEqual(1);
    expect(noButtons.length).toBeGreaterThanOrEqual(1);
  });

  it('enhance button disabled with no answers', async () => {
    mockEnhancePrompt.mockResolvedValueOnce(sampleQuestions);
    render(<PromptEnhanceModal {...defaultProps} />);

    await waitFor(() => {
      expect(screen.getByText('What programming language?')).toBeInTheDocument();
    });

    const enhanceBtn = screen.getByRole('button', { name: 'Enhance' });
    expect(enhanceBtn).toBeDisabled();
  });

  it('enhance button calls API with answers', async () => {
    mockEnhancePrompt
      .mockResolvedValueOnce(sampleQuestions)
      .mockResolvedValueOnce(sampleEnhanced);

    const user = userEvent.setup();
    render(<PromptEnhanceModal {...defaultProps} />);

    await waitFor(() => {
      expect(screen.getByText('What programming language?')).toBeInTheDocument();
    });

    // Answer all questions
    await user.click(screen.getByText('TypeScript'));
    await user.click(screen.getByText('QuickSort'));
    await user.click(screen.getByText('Yes'));

    const enhanceBtn = screen.getByRole('button', { name: 'Enhance' });
    expect(enhanceBtn).not.toBeDisabled();
    await user.click(enhanceBtn);

    await waitFor(() => {
      expect(mockEnhancePrompt).toHaveBeenCalledTimes(2);
    });

    const secondCall = mockEnhancePrompt.mock.calls[1][0];
    expect(secondCall.answers).toBeDefined();
    expect(secondCall.answers!.length).toBe(3);
    expect(secondCall.answers).toEqual([
      { questionId: 'q1', questionText: 'What programming language?', selectedOptions: ['TypeScript'] },
      { questionId: 'q2', questionText: 'What sorting algorithm?', selectedOptions: ['QuickSort'] },
      {
        questionId: 'q3',
        questionText: 'Should the function handle edge cases?',
        selectedOptions: ['Yes'],
      },
    ]);
  });

  it('result view shows enhanced prompt', async () => {
    mockEnhancePrompt
      .mockResolvedValueOnce(sampleQuestions)
      .mockResolvedValueOnce(sampleEnhanced);

    const user = userEvent.setup();
    render(<PromptEnhanceModal {...defaultProps} />);

    await waitFor(() => {
      expect(screen.getByText('What programming language?')).toBeInTheDocument();
    });

    await user.click(screen.getByText('TypeScript'));
    await user.click(screen.getByText('QuickSort'));
    await user.click(screen.getByText('Yes'));
    await user.click(screen.getByRole('button', { name: 'Enhance' }));

    await waitFor(() => {
      expect(screen.getByText(sampleEnhanced.enhancedPrompt!)).toBeInTheDocument();
    });
  });

  it('use this prompt calls onUsePrompt', async () => {
    mockEnhancePrompt
      .mockResolvedValueOnce(sampleQuestions)
      .mockResolvedValueOnce(sampleEnhanced);

    const onUsePrompt = vi.fn();
    const user = userEvent.setup();
    render(<PromptEnhanceModal {...defaultProps} onUsePrompt={onUsePrompt} />);

    await waitFor(() => {
      expect(screen.getByText('What programming language?')).toBeInTheDocument();
    });

    await user.click(screen.getByText('TypeScript'));
    await user.click(screen.getByText('QuickSort'));
    await user.click(screen.getByText('Yes'));
    await user.click(screen.getByRole('button', { name: 'Enhance' }));

    await waitFor(() => {
      expect(screen.getByText('Use this prompt')).toBeInTheDocument();
    });

    await user.click(screen.getByText('Use this prompt'));
    expect(onUsePrompt).toHaveBeenCalledWith(sampleEnhanced.enhancedPrompt);
  });

  it('copy button copies to clipboard', async () => {
    mockEnhancePrompt
      .mockResolvedValueOnce(sampleQuestions)
      .mockResolvedValueOnce(sampleEnhanced);

    const user = userEvent.setup();
    render(<PromptEnhanceModal {...defaultProps} />);

    await waitFor(() => {
      expect(screen.getByText('What programming language?')).toBeInTheDocument();
    });

    await user.click(screen.getByText('TypeScript'));
    await user.click(screen.getByText('QuickSort'));
    await user.click(screen.getByText('Yes'));
    await user.click(screen.getByRole('button', { name: 'Enhance' }));

    await waitFor(() => {
      expect(screen.getByText('Copy')).toBeInTheDocument();
    });

    // Install the clipboard spy right before clicking Copy, after userEvent.setup() has done its thing
    const writeTextSpy = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText: writeTextSpy },
      writable: true,
      configurable: true,
    });

    await user.click(screen.getByText('Copy'));

    expect(writeTextSpy).toHaveBeenCalledWith(sampleEnhanced.enhancedPrompt);

    await waitFor(() => {
      expect(screen.getByText('Copied!')).toBeInTheDocument();
    });
  });

  it('go deeper fires new question request', async () => {
    const deeperQuestions: EnhanceResponse = {
      questions: [
        {
          id: 'q4',
          text: 'What about error handling?',
          type: 'single_select',
          options: [
            { id: 'opt8', label: 'Throw errors' },
            { id: 'opt9', label: 'Return null' },
          ],
        },
      ],
    };

    mockEnhancePrompt
      .mockResolvedValueOnce(sampleQuestions)
      .mockResolvedValueOnce(sampleEnhanced)
      .mockResolvedValueOnce(deeperQuestions);

    const user = userEvent.setup();
    render(<PromptEnhanceModal {...defaultProps} />);

    await waitFor(() => {
      expect(screen.getByText('What programming language?')).toBeInTheDocument();
    });

    await user.click(screen.getByText('TypeScript'));
    await user.click(screen.getByText('QuickSort'));
    await user.click(screen.getByText('Yes'));
    await user.click(screen.getByRole('button', { name: 'Enhance' }));

    await waitFor(() => {
      expect(screen.getByText('Go Deeper')).toBeInTheDocument();
    });

    await user.click(screen.getByText('Go Deeper'));

    await waitFor(() => {
      expect(screen.getByText('What about error handling?')).toBeInTheDocument();
    });

    // The third call should have iteration=1
    expect(mockEnhancePrompt).toHaveBeenCalledTimes(3);
    const thirdCall = mockEnhancePrompt.mock.calls[2][0];
    expect(thirdCall.iteration).toBe(1);
    expect(thirdCall.previousEnhancedPrompt).toBe(sampleEnhanced.enhancedPrompt);
  });

  it('backdrop click closes modal', async () => {
    mockEnhancePrompt.mockResolvedValueOnce(sampleQuestions);
    const onClose = vi.fn();
    const user = userEvent.setup();
    render(<PromptEnhanceModal {...defaultProps} onClose={onClose} />);

    await waitFor(() => {
      expect(screen.getByText('What programming language?')).toBeInTheDocument();
    });

    // Click the backdrop (the outer div)
    const backdrop = document.querySelector('.enhance-modal-backdrop');
    expect(backdrop).toBeTruthy();
    await user.click(backdrop!);

    expect(onClose).toHaveBeenCalled();
  });

  it('escape key closes modal', async () => {
    mockEnhancePrompt.mockResolvedValueOnce(sampleQuestions);
    const onClose = vi.fn();
    const user = userEvent.setup();
    render(<PromptEnhanceModal {...defaultProps} onClose={onClose} />);

    await waitFor(() => {
      expect(screen.getByText('What programming language?')).toBeInTheDocument();
    });

    await user.keyboard('{Escape}');
    expect(onClose).toHaveBeenCalled();
  });

  it('error state shows retry', async () => {
    mockEnhancePrompt.mockRejectedValueOnce(new Error('Network error'));
    render(<PromptEnhanceModal {...defaultProps} />);

    await waitFor(() => {
      expect(screen.getByText('Network error')).toBeInTheDocument();
    });

    expect(screen.getByText('Retry')).toBeInTheDocument();
  });

  it('retry after error re-fetches questions', async () => {
    mockEnhancePrompt
      .mockRejectedValueOnce(new Error('Network error'))
      .mockResolvedValueOnce(sampleQuestions);

    const user = userEvent.setup();
    render(<PromptEnhanceModal {...defaultProps} />);

    await waitFor(() => {
      expect(screen.getByText('Network error')).toBeInTheDocument();
    });

    await user.click(screen.getByText('Retry'));

    await waitFor(() => {
      expect(screen.getByText('What programming language?')).toBeInTheDocument();
    });

    expect(mockEnhancePrompt).toHaveBeenCalledTimes(2);
  });

  it('does not render when isOpen is false', () => {
    mockEnhancePrompt.mockReturnValue(new Promise(() => {}));
    render(<PromptEnhanceModal {...defaultProps} isOpen={false} />);

    expect(screen.queryByText('Enhance Prompt')).not.toBeInTheDocument();
  });
});
