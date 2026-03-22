import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import App from "./App";
import * as api from "./lib/api";

const getMeMock = vi.fn();
const authWithGoogleMock = vi.fn();
const logoutMock = vi.fn();
const listModelsMock = vi.fn();
const updateModelPreferenceMock = vi.fn();
const updateModelFavoriteMock = vi.fn();
const updateModelReasoningPresetMock = vi.fn();
const createConversationMock = vi.fn();
const listConversationsMock = vi.fn();
const listConversationMessagesMock = vi.fn();
const deleteConversationMock = vi.fn();
const deleteAllConversationsMock = vi.fn();
const uploadFileMock = vi.fn();
const streamMessageMock = vi.fn();
const getAgentRunStatusMock = vi.fn();

beforeEach(() => {
  vi.restoreAllMocks();

  getMeMock.mockReset();
  authWithGoogleMock.mockReset();
  logoutMock.mockReset();
  listModelsMock.mockReset();
  updateModelPreferenceMock.mockReset();
  updateModelFavoriteMock.mockReset();
  updateModelReasoningPresetMock.mockReset();
  createConversationMock.mockReset();
  listConversationsMock.mockReset();
  listConversationMessagesMock.mockReset();
  deleteConversationMock.mockReset();
  deleteAllConversationsMock.mockReset();
  uploadFileMock.mockReset();
  streamMessageMock.mockReset();
  getAgentRunStatusMock.mockReset();

  getMeMock.mockResolvedValue({
    id: "user-1",
    email: "user@example.com",
    name: "User",
  });
  listModelsMock.mockResolvedValue({
    models: [
      {
        id: "openrouter/free",
        name: "OpenRouter Free",
        provider: "openrouter",
        contextWindow: 128000,
        promptPriceMicrosUsd: 0,
        outputPriceMicrosUsd: 0,
        supportsReasoning: true,
        curated: true,
      },
    ],
    curatedModels: [
      {
        id: "openrouter/free",
        name: "OpenRouter Free",
        provider: "openrouter",
        contextWindow: 128000,
        promptPriceMicrosUsd: 0,
        outputPriceMicrosUsd: 0,
        supportsReasoning: true,
        curated: true,
      },
    ],
    favorites: [],
    reasoningPresets: [
      { modelId: "openrouter/free", mode: "chat", effort: "medium" },
      { modelId: "openrouter/free", mode: "deep_research", effort: "high" },
    ],
    preferences: {
      lastUsedModelId: "openrouter/free",
      lastUsedDeepResearchModelId: "openrouter/free",
      lastUsedAgentModelId: "openrouter/free",
    },
  });
  updateModelPreferenceMock.mockResolvedValue({
    lastUsedModelId: "openrouter/free",
    lastUsedDeepResearchModelId: "openrouter/free",
    lastUsedAgentModelId: "openrouter/free",
  });
  updateModelFavoriteMock.mockResolvedValue([]);
  updateModelReasoningPresetMock.mockImplementation(
    async (
      _modelId: string,
      mode: api.ReasoningMode,
      effort: api.ReasoningEffort,
    ) => [
      { modelId: "openrouter/free", mode, effort },
      {
        modelId: "openrouter/free",
        mode: mode === "chat" ? "deep_research" : "chat",
        effort: mode === "chat" ? "high" : "medium",
      },
    ],
  );
  createConversationMock.mockResolvedValue({
    id: "conv-1",
    title: "New Chat",
    createdAt: "2026-02-10T00:00:00Z",
    updatedAt: "2026-02-10T00:00:00Z",
  });
  listConversationsMock.mockResolvedValue([]);
  listConversationMessagesMock.mockResolvedValue([]);
  deleteConversationMock.mockResolvedValue(undefined);
  deleteAllConversationsMock.mockResolvedValue(undefined);
  authWithGoogleMock.mockResolvedValue({
    id: "user-1",
    email: "user@example.com",
  });
  logoutMock.mockResolvedValue(undefined);
  uploadFileMock.mockResolvedValue({
    id: "file-1",
    filename: "notes.md",
    mediaType: "text/markdown",
    sizeBytes: 100,
    createdAt: "2026-02-10T00:00:00Z",
  });
  streamMessageMock.mockResolvedValue(undefined);
  getAgentRunStatusMock.mockResolvedValue({ id: "run-1", status: "running" });

  vi.spyOn(api, "getMe").mockImplementation(getMeMock);
  vi.spyOn(api, "authWithGoogle").mockImplementation(authWithGoogleMock);
  vi.spyOn(api, "logout").mockImplementation(logoutMock);
  vi.spyOn(api, "listModels").mockImplementation(listModelsMock);
  vi.spyOn(api, "updateModelPreference").mockImplementation(
    updateModelPreferenceMock,
  );
  vi.spyOn(api, "updateModelFavorite").mockImplementation(
    updateModelFavoriteMock,
  );
  vi.spyOn(api, "updateModelReasoningPreset").mockImplementation(
    updateModelReasoningPresetMock,
  );
  vi.spyOn(api, "createConversation").mockImplementation(
    createConversationMock,
  );
  vi.spyOn(api, "listConversations").mockImplementation(listConversationsMock);
  vi.spyOn(api, "listConversationMessages").mockImplementation(
    listConversationMessagesMock,
  );
  vi.spyOn(api, "deleteConversation").mockImplementation(
    deleteConversationMock,
  );
  vi.spyOn(api, "deleteAllConversations").mockImplementation(
    deleteAllConversationsMock,
  );
  vi.spyOn(api, "uploadFile").mockImplementation(uploadFileMock);
  vi.spyOn(api, "streamMessage").mockImplementation(streamMessageMock);
  vi.spyOn(api, "getAgentRunStatus").mockImplementation(getAgentRunStatusMock);
});

afterEach(() => {
  vi.useRealTimers();
  cleanup();
});

describe("Deep research streaming UX", () => {
  it("keeps first message visible and shows thinking UI before conversation metadata arrives", async () => {
    let releaseStream: (() => void) | undefined;
    streamMessageMock.mockImplementation(
      async () =>
        await new Promise<void>((resolve) => {
          releaseStream = () => resolve();
        }),
    );

    const user = userEvent.setup();
    render(<App />);

    await screen.findByPlaceholderText("Ask anything...");

    await user.click(screen.getByRole("button", { name: /new conversation/i }));
    await user.type(
      screen.getByPlaceholderText("Ask anything..."),
      "First prompt",
    );
    await user.click(screen.getAllByRole("button", { name: /send/i })[0]);

    await waitFor(() => {
      expect(streamMessageMock).toHaveBeenCalledTimes(1);
    });

    expect(screen.getByText("First prompt")).toBeInTheDocument();
    expect(
      screen.getByText("Thinking", { selector: ".thinking-status-chip-label" }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /thought process/i }),
    ).not.toBeInTheDocument();

    if (typeof releaseStream === "function") {
      releaseStream();
    }

    await waitFor(() => {
      expect(
        screen.queryByRole("button", { name: /^stop$/i }),
      ).not.toBeInTheDocument();
    });
  });

  it("adds a conversation only after first send starts streaming", async () => {
    const createdConversation: api.Conversation = {
      id: "conv-new",
      title: "New Chat",
      createdAt: "2026-02-10T00:00:00Z",
      updatedAt: "2026-02-10T00:00:00Z",
    };

    let listCalls = 0;
    listConversationsMock.mockImplementation(async () => {
      listCalls += 1;
      if (listCalls === 1) return [];
      return [createdConversation];
    });

    streamMessageMock.mockImplementation(
      async (
        _request: api.ChatRequest,
        onEvent: (event: api.StreamEvent) => void,
      ) => {
        onEvent({
          type: "metadata",
          grounding: true,
          deepResearch: false,
          modelId: "openrouter/free",
          conversationId: "conv-new",
        });
        onEvent({ type: "token", delta: "Assistant reply" });
        onEvent({ type: "done" });
      },
    );

    const user = userEvent.setup();
    render(<App />);

    await screen.findByText("No conversations yet");

    await user.click(screen.getByRole("button", { name: /new conversation/i }));

    expect(createConversationMock).not.toHaveBeenCalled();
    await screen.findByText("No conversations yet");

    await user.type(
      screen.getByPlaceholderText("Ask anything..."),
      "Start now",
    );
    await user.click(screen.getAllByRole("button", { name: /send/i })[0]);

    await waitFor(() => {
      expect(streamMessageMock).toHaveBeenCalledTimes(1);
    });
    expect(streamMessageMock.mock.calls[0][0]).toMatchObject({
      message: "Start now",
      conversationId: undefined,
    });

    await waitFor(() => {
      expect(screen.getByText("New Chat")).toBeInTheDocument();
    });
  });

  it("collapses sidebar when starting a new conversation", async () => {
    const user = userEvent.setup();
    const { container } = render(<App />);

    await screen.findByPlaceholderText("Ask anything...");

    const sidebar = container.querySelector("aside.sidebar");
    expect(sidebar).not.toBeNull();
    expect(sidebar).toHaveClass("collapsed");

    await user.click(screen.getByRole("button", { name: /toggle sidebar/i }));
    expect(sidebar).not.toHaveClass("collapsed");

    await user.click(screen.getByRole("button", { name: /new conversation/i }));

    expect(sidebar).toHaveClass("collapsed");
  });

  it("renders research phases and completion state from progress events", async () => {
    let releaseStream: (() => void) | undefined;
    streamMessageMock.mockImplementation(
      async (
        _request: api.ChatRequest,
        onEvent: (event: api.StreamEvent) => void,
      ) => {
        onEvent({
          type: "metadata",
          grounding: true,
          deepResearch: true,
          modelId: "openrouter/free",
          conversationId: "conv-1",
        });
        onEvent({
          type: "progress",
          phase: "planning",
          title: "Mapping the request",
          detail:
            "Figuring out what needs to be answered before drafting anything.",
          totalPasses: 3,
          loop: 1,
          maxLoops: 3,
        });
        onEvent({
          type: "progress",
          phase: "searching",
          title: "Scanning for trustworthy sources",
          detail:
            "Looking for current, relevant sources that can support the answer.",
          pass: 1,
          totalPasses: 3,
          loop: 1,
          maxLoops: 3,
        });
        onEvent({
          type: "progress",
          phase: "reading",
          title: "Pulling facts from the strongest results",
          detail:
            "Reading the most promising pages and extracting usable evidence.",
          loop: 1,
          maxLoops: 3,
          sourcesConsidered: 4,
          sourcesRead: 2,
        });
        onEvent({
          type: "progress",
          phase: "evaluating",
          title: "Comparing what holds up",
          detail:
            "Checking whether the sources agree and whether the evidence is strong enough.",
          loop: 1,
          maxLoops: 3,
          sourcesConsidered: 4,
          sourcesRead: 2,
        });
        onEvent({
          type: "progress",
          phase: "iterating",
          title: "Closing the remaining gaps",
          detail:
            "Running another pass where the answer still needs more support.",
          loop: 1,
          maxLoops: 3,
          sourcesConsidered: 4,
          sourcesRead: 2,
        });
        onEvent({
          type: "progress",
          phase: "synthesizing",
          title: "Shaping the response",
          detail:
            "Turning the research into a clear response with grounded claims.",
        });
        onEvent({ type: "token", delta: "Final answer [1]." });
        onEvent({
          type: "progress",
          phase: "finalizing",
          title: "Polishing the final answer",
          detail:
            "Tightening wording, organizing citations, and preparing the response.",
        });
        await new Promise<void>((resolve) => {
          releaseStream = () => {
            onEvent({ type: "done" });
            resolve();
          };
        });
      },
    );

    const user = userEvent.setup();
    render(<App />);

    await screen.findByPlaceholderText("Ask anything...");

    await user.click(screen.getByRole("button", { name: /deep research/i }));
    await user.type(
      screen.getByPlaceholderText("Ask anything..."),
      "Need deep research output",
    );
    await user.click(screen.getAllByRole("button", { name: /send/i })[0]);

    await waitFor(() => {
      expect(streamMessageMock).toHaveBeenCalledTimes(1);
    });

    expect(screen.queryByTestId("research-timeline")).not.toBeInTheDocument();
    expect(
      screen.getByText(
        "Tightening wording, organizing citations, and preparing the response.",
      ),
    ).toBeInTheDocument();
    expect(
      screen.getByText("Thinking", { selector: ".thinking-status-chip-label" }),
    ).toBeInTheDocument();

    // Expand the in-message panel to see all phases
    await user.click(screen.getByRole("button", { name: /thinking/i }));

    expect(screen.getByText("Mapping the request")).toBeInTheDocument();
    expect(
      screen.getByText("Scanning for trustworthy sources"),
    ).toBeInTheDocument();
    expect(
      screen.getByText("Pulling facts from the strongest results"),
    ).toBeInTheDocument();
    expect(screen.getByText("Comparing what holds up")).toBeInTheDocument();
    expect(screen.getByText("Closing the remaining gaps")).toBeInTheDocument();
    expect(screen.getByText("Shaping the response")).toBeInTheDocument();
    expect(screen.getByText("Polishing the final answer")).toBeInTheDocument();

    if (releaseStream) releaseStream();
  });

  it("renders usage details after sources when usage events are streamed", async () => {
    let releaseStream: (() => void) | undefined;
    streamMessageMock.mockImplementation(
      async (
        _request: api.ChatRequest,
        onEvent: (event: api.StreamEvent) => void,
      ) => {
        onEvent({
          type: "metadata",
          grounding: true,
          deepResearch: false,
          modelId: "openrouter/free",
        });
        onEvent({ type: "token", delta: "Answer with source [1]." });
        onEvent({
          type: "citations",
          citations: [
            { url: "https://example.com/source", title: "Example Source" },
          ],
        });
        onEvent({
          type: "usage",
          usage: {
            promptTokens: 120,
            completionTokens: 48,
            totalTokens: 168,
            costMicrosUsd: 420,
            byokInferenceCostMicrosUsd: 111,
            tokensPerSecond: 24.5,
            modelId: "openai/gpt-4o-mini",
            providerName: "OpenAI",
          },
        });
        await new Promise<void>((resolve) => {
          releaseStream = () => {
            onEvent({ type: "done" });
            resolve();
          };
        });
      },
    );

    const user = userEvent.setup();
    render(<App />);

    await screen.findByPlaceholderText("Ask anything...");
    await user.type(
      screen.getByPlaceholderText("Ask anything..."),
      "Show usage",
    );
    await user.click(screen.getAllByRole("button", { name: /send/i })[0]);

    await waitFor(() => {
      expect(streamMessageMock).toHaveBeenCalledTimes(1);
    });

    const sourcesButton = await screen.findByRole("button", {
      name: /sources/i,
    });
    const usageButton = await screen.findByRole("button", { name: /usage/i });
    expect(
      (sourcesButton.compareDocumentPosition(usageButton) &
        Node.DOCUMENT_POSITION_FOLLOWING) !==
        0,
    ).toBe(true);
    expect(screen.getByText("$0.000531 / 24.50 tok/s")).toBeInTheDocument();

    await user.click(usageButton);
    expect(screen.getByText("Model")).toBeInTheDocument();
    expect(screen.getByText("openai/gpt-4o-mini")).toBeInTheDocument();
    expect(screen.getByText("Provider")).toBeInTheDocument();
    expect(screen.getByText("OpenAI")).toBeInTheDocument();
    expect(screen.getByText("Input tokens")).toBeInTheDocument();
    expect(screen.getByText("120")).toBeInTheDocument();
    expect(screen.getByText("$0.000420")).toBeInTheDocument();
    expect(screen.getByText("$0.000111")).toBeInTheDocument();
    expect(screen.getByText("24.50 tok/s")).toBeInTheDocument();

    if (releaseStream) releaseStream();
  });

  it("renders compact one-line progress for non-deep-research sends", async () => {
    let releaseStream: (() => void) | undefined;
    streamMessageMock.mockImplementation(
      async (
        _request: api.ChatRequest,
        onEvent: (event: api.StreamEvent) => void,
      ) => {
        onEvent({
          type: "metadata",
          grounding: true,
          deepResearch: false,
          modelId: "openrouter/free",
          conversationId: "conv-1",
        });
        onEvent({
          type: "progress",
          phase: "searching",
          title: "Finding an initial source",
          isQuickStep: true,
        });
        await new Promise<void>((resolve) => {
          releaseStream = () => {
            onEvent({ type: "done" });
            resolve();
          };
        });
      },
    );

    const user = userEvent.setup();
    render(<App />);

    await screen.findByPlaceholderText("Ask anything...");
    await user.type(
      screen.getByPlaceholderText("Ask anything..."),
      "Normal chat request",
    );
    await user.click(screen.getAllByRole("button", { name: /send/i })[0]);

    await waitFor(() => {
      expect(streamMessageMock).toHaveBeenCalledTimes(1);
    });

    expect(screen.queryByTestId("research-timeline")).not.toBeInTheDocument();
    expect(screen.getByText("Finding an initial source")).toBeInTheDocument();
    expect(
      screen.queryByText(
        "Looking for current, relevant sources that can support the answer.",
      ),
    ).not.toBeInTheDocument();

    if (releaseStream) releaseStream();
  });

  it("keeps model reasoning collapsed by default and expands it on demand", async () => {
    let releaseStream: (() => void) | undefined;
    streamMessageMock.mockImplementation(
      async (
        _request: api.ChatRequest,
        onEvent: (event: api.StreamEvent) => void,
      ) => {
        onEvent({
          type: "metadata",
          grounding: true,
          deepResearch: false,
          modelId: "openrouter/free",
          conversationId: "conv-1",
        });
        onEvent({
          type: "progress",
          phase: "planning",
          title: "Mapping the request",
          detail:
            "Figuring out what needs to be answered before drafting anything.",
        });
        onEvent({
          type: "reasoning",
          delta: "I am validating evidence consistency.",
        });
        await new Promise<void>((resolve) => {
          releaseStream = () => {
            onEvent({ type: "done" });
            resolve();
          };
        });
      },
    );

    const user = userEvent.setup();
    render(<App />);

    await screen.findByPlaceholderText("Ask anything...");
    await user.type(
      screen.getByPlaceholderText("Ask anything..."),
      "Show reasoning section",
    );
    await user.click(screen.getAllByRole("button", { name: /send/i })[0]);

    await waitFor(() => {
      expect(streamMessageMock).toHaveBeenCalledTimes(1);
    });

    await user.click(screen.getByRole("button", { name: /thinking/i }));

    expect(screen.getByText("Model reasoning")).toBeInTheDocument();
    expect(
      screen.queryByText("I am validating evidence consistency."),
    ).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /model reasoning/i }));

    expect(
      screen.getByText("I am validating evidence consistency."),
    ).toBeInTheDocument();

    if (releaseStream) releaseStream();
  });

  it("renders persisted thinking traces from conversation history", async () => {
    const existingConversation: api.Conversation = {
      id: "conv-existing",
      title: "Existing Chat",
      createdAt: "2026-02-10T00:00:00Z",
      updatedAt: "2026-02-10T00:00:00Z",
    };

    listConversationsMock.mockResolvedValue([existingConversation]);
    listConversationMessagesMock.mockResolvedValue([
      {
        id: "msg-user",
        conversationId: "conv-existing",
        role: "user",
        content: "What changed?",
        groundingEnabled: true,
        deepResearchEnabled: false,
        citations: [],
        createdAt: "2026-02-10T00:00:00Z",
      },
      {
        id: "msg-assistant",
        conversationId: "conv-existing",
        role: "assistant",
        content: "Here is the answer.",
        reasoningContent: "Checking release note chronology.",
        thinkingTrace: {
          status: "done",
          summary:
            "Tightening wording, organizing citations, and preparing the response.",
          entries: [
            {
              phase: "planning",
              title: "Mapping the request",
              detail:
                "Figuring out what needs to be answered before drafting anything.",
            },
          ],
        },
        groundingEnabled: true,
        deepResearchEnabled: false,
        citations: [],
        createdAt: "2026-02-10T00:00:01Z",
      },
    ]);

    const user = userEvent.setup();
    render(<App />);

    await screen.findByText("Existing Chat");
    await user.click(screen.getByText("Existing Chat"));

    await waitFor(() => {
      expect(listConversationMessagesMock).toHaveBeenCalledWith(
        "conv-existing",
      );
    });
    expect(
      screen.getByText(
        "Tightening wording, organizing citations, and preparing the response.",
      ),
    ).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /ready/i }));
    expect(screen.getByText("Mapping the request")).toBeInTheDocument();
  });

  it("includes selected reasoning effort in chat requests when model supports reasoning", async () => {
    streamMessageMock.mockImplementation(
      async (
        _request: api.ChatRequest,
        onEvent: (event: api.StreamEvent) => void,
      ) => {
        onEvent({
          type: "metadata",
          grounding: true,
          deepResearch: false,
          modelId: "openrouter/free",
          conversationId: "conv-1",
        });
        onEvent({ type: "done" });
      },
    );

    const user = userEvent.setup();
    render(<App />);

    await screen.findByPlaceholderText("Ask anything...");
    await user.selectOptions(screen.getByLabelText("Thinking effort"), "high");
    await user.type(
      screen.getByPlaceholderText("Ask anything..."),
      "Use higher effort",
    );
    await user.click(screen.getAllByRole("button", { name: /send/i })[0]);

    await waitFor(() => {
      expect(streamMessageMock).toHaveBeenCalledTimes(1);
    });

    expect(streamMessageMock.mock.calls[0][0]).toMatchObject({
      reasoningEffort: "high",
    });
  });

  it("sends mode=agent, keeps grounding on, and leaves the placeholder running after queue confirmation", async () => {
    streamMessageMock.mockImplementation(
      async (
        _request: api.ChatRequest,
        onEvent: (event: api.StreamEvent) => void,
      ) => {
        onEvent({
          type: "metadata",
          grounding: true,
          deepResearch: false,
          responseMode: "agent",
          modelId: "openrouter/free",
          conversationId: "conv-1",
        });
        onEvent({
          type: "progress",
          phase: "planning",
          title: "Queueing agent run",
          detail: "Preparing multi-agent workflow",
        });
        onEvent({ type: "done" });
      },
    );

    const user = userEvent.setup();
    render(<App />);

    await screen.findByPlaceholderText("Ask anything...");

    const groundingButton = screen.getByRole("button", { name: /grounding/i });
    await user.click(screen.getByRole("button", { name: /agent/i }));
    expect(groundingButton).toBeDisabled();

    await user.type(
      screen.getByPlaceholderText("Ask anything..."),
      "Run the agent workflow",
    );
    await user.click(screen.getAllByRole("button", { name: /send/i })[0]);

    await waitFor(() => {
      expect(streamMessageMock).toHaveBeenCalledTimes(1);
    });

    expect(streamMessageMock.mock.calls[0][0]).toMatchObject({
      mode: "agent",
      grounding: true,
      deepResearch: false,
    });
  });

  it("stops auto-scrolling when user scrolls up while streaming", async () => {
    let emitEvent: ((event: api.StreamEvent) => void) | null = null;
    let releaseStream: (() => void) | undefined;

    streamMessageMock.mockImplementation(
      async (
        _request: api.ChatRequest,
        onEvent: (event: api.StreamEvent) => void,
      ) => {
        emitEvent = onEvent;
        onEvent({
          type: "metadata",
          grounding: true,
          deepResearch: false,
          modelId: "openrouter/free",
        });
        onEvent({ type: "token", delta: "hello" });
        await new Promise<void>((resolve) => {
          releaseStream = resolve;
        });
      },
    );

    const user = userEvent.setup();
    const { container } = render(<App />);

    await screen.findByPlaceholderText("Ask anything...");

    const messagesContainer = container.querySelector<HTMLDivElement>(
      ".messages-container",
    );
    expect(messagesContainer).not.toBeNull();
    if (!messagesContainer) return;

    Object.defineProperty(messagesContainer, "scrollHeight", {
      configurable: true,
      value: 2000,
    });
    Object.defineProperty(messagesContainer, "clientHeight", {
      configurable: true,
      value: 600,
    });
    Object.defineProperty(messagesContainer, "scrollTop", {
      configurable: true,
      writable: true,
      value: 1400,
    });

    const scrollToSpy = vi.spyOn(messagesContainer, "scrollTo");

    await user.type(
      screen.getByPlaceholderText("Ask anything..."),
      "Stream test",
    );
    await user.click(screen.getAllByRole("button", { name: /send/i })[0]);

    await waitFor(() => {
      expect(streamMessageMock).toHaveBeenCalledTimes(1);
    });

    await waitFor(() => {
      expect(scrollToSpy.mock.calls.length).toBeGreaterThan(0);
    });

    const callsBeforeScrollUp = scrollToSpy.mock.calls.length;

    messagesContainer.scrollTop = 700;
    messagesContainer.dispatchEvent(new Event("scroll"));

    act(() => {
      emitEvent?.({ type: "token", delta: " world" });
    });

    await waitFor(() => {
      expect(screen.getByText("hello world")).toBeInTheDocument();
    });

    expect(scrollToSpy.mock.calls.length).toBe(callsBeforeScrollUp);

    if (releaseStream) {
      releaseStream();
    }
    await waitFor(() => {
      expect(
        screen.queryByRole("button", { name: /^stop$/i }),
      ).not.toBeInTheDocument();
    });
  });
});

describe("User message edit + resend", () => {
  it("sends editMessageId, truncates future messages, and renders regenerated answer", async () => {
    const createdConversation: api.Conversation = {
      id: "conv-1",
      title: "New Chat",
      createdAt: "2026-02-10T00:00:00Z",
      updatedAt: "2026-02-10T00:00:00Z",
    };
    let persistedMessages: api.ConversationMessage[] = [];
    listConversationsMock.mockResolvedValue([createdConversation]);
    listConversationMessagesMock.mockImplementation(
      async () => persistedMessages,
    );

    let streamCallCount = 0;
    streamMessageMock.mockImplementation(
      async (
        request: api.ChatRequest,
        onEvent: (event: api.StreamEvent) => void,
      ) => {
        streamCallCount += 1;
        const nextUserMessageID =
          streamCallCount === 1
            ? "user-1"
            : streamCallCount === 2
              ? "user-2"
              : "user-3";
        const assistantContent =
          streamCallCount === 1
            ? "First answer"
            : streamCallCount === 2
              ? "Second answer"
              : "Edited answer";

        if (request.editMessageId) {
          const editIndex = persistedMessages.findIndex(
            (message) => message.id === request.editMessageId,
          );
          persistedMessages =
            editIndex >= 0
              ? persistedMessages.slice(0, editIndex)
              : persistedMessages;
        }
        persistedMessages = [
          ...persistedMessages,
          {
            id: nextUserMessageID,
            conversationId: createdConversation.id,
            role: "user",
            content: request.message,
            groundingEnabled: true,
            deepResearchEnabled: false,
            citations: [],
            createdAt: "2026-02-10T00:00:00Z",
          },
          {
            id: `assistant-${streamCallCount}`,
            conversationId: createdConversation.id,
            role: "assistant",
            content: assistantContent,
            groundingEnabled: true,
            deepResearchEnabled: false,
            citations: [],
            createdAt: "2026-02-10T00:00:00Z",
          },
        ];

        if (streamCallCount === 3) {
          expect(request.editMessageId).toBe("user-2");
          expect(request.message).toBe("Second prompt edited");
          expect(request.conversationId).toBe("conv-1");
        }

        onEvent({
          type: "metadata",
          grounding: true,
          deepResearch: false,
          modelId: "openrouter/free",
          conversationId: "conv-1",
          userMessageId: nextUserMessageID,
        });
        onEvent({ type: "token", delta: assistantContent });
        onEvent({ type: "done" });
      },
    );

    const user = userEvent.setup();
    render(<App />);

    await screen.findByPlaceholderText("Ask anything...");

    await user.type(
      screen.getByPlaceholderText("Ask anything..."),
      "First prompt",
    );
    await user.click(screen.getAllByRole("button", { name: /send/i })[0]);
    await waitFor(() => {
      expect(streamMessageMock).toHaveBeenCalledTimes(1);
    });
    await screen.findByText("First answer");

    await user.type(
      screen.getByPlaceholderText("Ask anything..."),
      "Second prompt",
    );
    await user.click(screen.getAllByRole("button", { name: /send/i })[0]);
    await waitFor(() => {
      expect(streamMessageMock).toHaveBeenCalledTimes(2);
    });
    await screen.findByText("Second answer");

    const editButtons = screen.getAllByRole("button", {
      name: /edit message/i,
    });
    await user.click(editButtons[1]);

    const editTextarea = screen.getByRole("textbox", { name: /edit message/i });
    await user.clear(editTextarea);
    await user.type(editTextarea, "Second prompt edited");
    await user.click(screen.getByRole("button", { name: /save & resend/i }));

    await waitFor(() => {
      expect(streamMessageMock).toHaveBeenCalledTimes(3);
    });
    await screen.findByText("Edited answer");

    expect(screen.getByText("First prompt")).toBeInTheDocument();
    expect(screen.queryByText("Second answer")).not.toBeInTheDocument();
    expect(screen.queryByText(/^Second prompt$/)).not.toBeInTheDocument();
    expect(screen.getByText("Second prompt edited")).toBeInTheDocument();
  });
});

describe("Model selector filtering", () => {
  it("shows an always-visible copy icon on user messages and copies the sent text", async () => {
    const writeTextMock = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", {
      value: { writeText: writeTextMock },
      configurable: true,
    });

    const createdConversation: api.Conversation = {
      id: "conv-1",
      title: "New Chat",
      createdAt: "2026-02-10T00:00:00Z",
      updatedAt: "2026-02-10T00:00:00Z",
    };
    let listCalls = 0;
    listConversationsMock.mockImplementation(async () => {
      listCalls += 1;
      if (listCalls === 1) return [];
      return [createdConversation];
    });

    let releaseStream: (() => void) | undefined;
    streamMessageMock.mockImplementation(
      async (
        _request: api.ChatRequest,
        onEvent: (event: api.StreamEvent) => void,
      ) => {
        onEvent({
          type: "metadata",
          grounding: true,
          deepResearch: false,
          modelId: "openrouter/free",
          conversationId: "conv-1",
        });
        await new Promise<void>((resolve) => {
          releaseStream = () => {
            onEvent({ type: "done" });
            resolve();
          };
        });
      },
    );

    const user = userEvent.setup();
    render(<App />);

    await screen.findByPlaceholderText("Ask anything...");

    await user.type(
      screen.getByPlaceholderText("Ask anything..."),
      "Copy this user message",
    );
    await user.click(screen.getAllByRole("button", { name: /send/i })[0]);

    await waitFor(() => {
      expect(streamMessageMock).toHaveBeenCalledTimes(1);
    });

    const copyButton = await screen.findByRole("button", {
      name: /copy message/i,
    });
    expect(copyButton).toBeInTheDocument();

    await user.click(copyButton);

    expect(copyButton).toHaveAttribute("aria-label", "Message copied");

    if (releaseStream) releaseStream();
    await waitFor(() => {
      expect(
        screen.queryByRole("button", { name: /^stop$/i }),
      ).not.toBeInTheDocument();
    });
  });

  it("keeps free + favorites visible when All is off and shows the rest when toggled on", async () => {
    listModelsMock.mockResolvedValueOnce({
      models: [
        {
          id: "openrouter/free",
          name: "OpenRouter Free",
          provider: "openrouter",
          contextWindow: 128000,
          promptPriceMicrosUsd: 0,
          outputPriceMicrosUsd: 0,
          curated: true,
        },
        {
          id: "openrouter/latest-used",
          name: "Latest Used Model",
          provider: "openrouter",
          contextWindow: 128000,
          promptPriceMicrosUsd: 25,
          outputPriceMicrosUsd: 30,
          curated: false,
        },
        {
          id: "openrouter/favorite-model",
          name: "Favorite Model",
          provider: "openrouter",
          contextWindow: 200000,
          promptPriceMicrosUsd: 35,
          outputPriceMicrosUsd: 45,
          curated: false,
        },
        {
          id: "openrouter/other-model",
          name: "Other Model",
          provider: "openrouter",
          contextWindow: 64000,
          promptPriceMicrosUsd: 10,
          outputPriceMicrosUsd: 12,
          curated: false,
        },
      ],
      curatedModels: [
        {
          id: "openrouter/free",
          name: "OpenRouter Free",
          provider: "openrouter",
          contextWindow: 128000,
          promptPriceMicrosUsd: 0,
          outputPriceMicrosUsd: 0,
          curated: true,
        },
      ],
      favorites: ["openrouter/favorite-model"],
      preferences: {
        lastUsedModelId: "openrouter/latest-used",
        lastUsedDeepResearchModelId: "openrouter/latest-used",
        lastUsedAgentModelId: "openrouter/latest-used",
      },
    });

    const user = userEvent.setup();
    render(<App />);

    await screen.findByPlaceholderText("Ask anything...");

    await user.click(screen.getByRole("button", { name: "Latest Used Model" }));

    expect(screen.getByText("OpenRouter Free")).toBeInTheDocument();
    expect(screen.getByText("Favorite Model")).toBeInTheDocument();
    expect(screen.queryByText("Other Model")).not.toBeInTheDocument();

    await user.click(screen.getByRole("switch", { name: /show all models/i }));

    expect(screen.getByText("Other Model")).toBeInTheDocument();
  });

  it("shows context and pricing metadata using compact ctx with input and output pricing", async () => {
    listModelsMock.mockResolvedValueOnce({
      models: [
        {
          id: "openrouter/million-context",
          name: "Million Context Model",
          provider: "openrouter",
          contextWindow: 1_000_000,
          promptPriceMicrosUsd: 25,
          outputPriceMicrosUsd: 30,
          curated: true,
        },
        {
          id: "openrouter/compact-context",
          name: "Compact Context Model",
          provider: "openrouter",
          contextWindow: 256_000,
          promptPriceMicrosUsd: 10,
          outputPriceMicrosUsd: 12,
          curated: true,
        },
      ],
      curatedModels: [
        {
          id: "openrouter/million-context",
          name: "Million Context Model",
          provider: "openrouter",
          contextWindow: 1_000_000,
          promptPriceMicrosUsd: 25,
          outputPriceMicrosUsd: 30,
          curated: true,
        },
        {
          id: "openrouter/compact-context",
          name: "Compact Context Model",
          provider: "openrouter",
          contextWindow: 256_000,
          promptPriceMicrosUsd: 10,
          outputPriceMicrosUsd: 12,
          curated: true,
        },
      ],
      favorites: [],
      preferences: {
        lastUsedModelId: "openrouter/million-context",
        lastUsedDeepResearchModelId: "openrouter/million-context",
        lastUsedAgentModelId: "openrouter/million-context",
      },
    });

    const user = userEvent.setup();
    render(<App />);

    await screen.findByPlaceholderText("Ask anything...");
    await user.click(
      screen.getByRole("button", { name: "Million Context Model" }),
    );

    expect(
      screen.getByText(
        (content) =>
          content.includes("1M ctx") &&
          content.includes("25.00$ In - 30.00$ Out"),
      ),
    ).toBeInTheDocument();
    expect(
      screen.getByText(
        (content) =>
          content.includes("256K ctx") &&
          content.includes("10.00$ In - 12.00$ Out"),
      ),
    ).toBeInTheDocument();
  });

  it("sends council sourceModels and fusionModel in the request payload", async () => {
    listModelsMock.mockResolvedValueOnce({
      models: [
        {
          id: "openrouter/free",
          name: "OpenRouter Free",
          provider: "openrouter",
          contextWindow: 128000,
          promptPriceMicrosUsd: 0,
          outputPriceMicrosUsd: 0,
          supportsReasoning: true,
          curated: true,
        },
        {
          id: "source-a",
          name: "Source Alpha",
          provider: "openrouter",
          contextWindow: 128000,
          promptPriceMicrosUsd: 0,
          outputPriceMicrosUsd: 0,
          supportsReasoning: true,
          curated: true,
        },
        {
          id: "fusion-model",
          name: "Fusion Model",
          provider: "openrouter",
          contextWindow: 128000,
          promptPriceMicrosUsd: 0,
          outputPriceMicrosUsd: 0,
          supportsReasoning: true,
          curated: true,
        },
      ],
      curatedModels: [],
      favorites: [],
      reasoningPresets: [
        { modelId: "openrouter/free", mode: "chat", effort: "medium" },
        { modelId: "openrouter/free", mode: "deep_research", effort: "high" },
        { modelId: "openrouter/free", mode: "agent", effort: "medium" },
      ],
      preferences: {
        lastUsedModelId: "openrouter/free",
        lastUsedDeepResearchModelId: "openrouter/free",
        lastUsedAgentModelId: "openrouter/free",
      },
    });

    const user = userEvent.setup();
    const { container } = render(<App />);

    await screen.findByPlaceholderText("Ask anything...");
    await user.click(screen.getByRole("button", { name: /agent/i }));

    const initialCouncilSelects = container.querySelectorAll(
      "select.council-select",
    );
    expect(initialCouncilSelects.length).toBe(1);
    await user.selectOptions(initialCouncilSelects[0], "source-a");

    const councilSelects = container.querySelectorAll("select.council-select");
    expect(councilSelects.length).toBe(2);
    await user.selectOptions(councilSelects[1], "fusion-model");

    await user.type(
      screen.getByPlaceholderText("Ask anything..."),
      "Run council mode",
    );
    await user.click(screen.getAllByRole("button", { name: /send/i })[0]);

    await waitFor(() => {
      expect(streamMessageMock).toHaveBeenCalledTimes(1);
    });

    expect(streamMessageMock.mock.calls[0][0]).toMatchObject({
      mode: "agent",
      sourceModels: [{ modelId: "source-a", reasoningEffort: "medium" }],
      fusionModel: { modelId: "fusion-model", reasoningEffort: "medium" },
    });
  });

  it("stops council polling and marks the trace done when the run completes", async () => {
    const existingConversation: api.Conversation = {
      id: "conv-1",
      title: "Council Chat",
      createdAt: "2026-02-10T00:00:00Z",
      updatedAt: "2026-02-10T00:00:00Z",
    };

    listConversationsMock.mockResolvedValue([existingConversation]);
    listConversationMessagesMock.mockResolvedValue([
      {
        id: "msg-user",
        conversationId: "conv-1",
        role: "user",
        content: "Poll council status",
        groundingEnabled: false,
        deepResearchEnabled: false,
        citations: [],
        createdAt: "2026-02-10T00:00:00Z",
      },
      {
        id: "msg-assistant",
        conversationId: "conv-1",
        role: "assistant",
        content: "",
        responseMode: "agent",
        agentRunId: "run-1",
        thinkingTrace: {
          status: "running",
          summary: "Coordinating the council workflow",
          entries: [{ phase: "planning", title: "Starting council workflow" }],
        },
        groundingEnabled: false,
        deepResearchEnabled: false,
        citations: [],
        createdAt: "2026-02-10T00:00:01Z",
      },
    ]);
    getAgentRunStatusMock.mockResolvedValue({
      id: "run-1",
      status: "completed",
      sourceResults: [
        {
          modelId: "source-a",
          status: "complete",
          readableSources: 15,
          response: "Source answer",
        },
      ],
      result: { modelId: "fusion-model", response: "Final council answer" },
    });
    streamMessageMock.mockImplementation(
      async (
        _request: api.ChatRequest,
        onEvent: (event: api.StreamEvent) => void,
      ) => {
        onEvent({
          type: "metadata",
          grounding: false,
          deepResearch: false,
          responseMode: "agent",
          modelId: "fusion-model",
          conversationId: "conv-1",
          agentRunId: "run-1",
        });
        onEvent({ type: "done" });
      },
    );

    const user = userEvent.setup();
    render(<App />);

    await screen.findByPlaceholderText("Ask anything...");
    await user.type(
      screen.getByPlaceholderText("Ask anything..."),
      "Poll council status",
    );
    await user.click(screen.getAllByRole("button", { name: /send/i })[0]);

    await waitFor(() => {
      expect(streamMessageMock).toHaveBeenCalledTimes(1);
    });

    await act(async () => {
      await new Promise((resolve) => window.setTimeout(resolve, 2200));
    });

    await waitFor(() => {
      expect(getAgentRunStatusMock).toHaveBeenCalledWith("run-1");
    });
    expect(screen.getByText("Final council answer")).toBeInTheDocument();
    expect(screen.getByText("Council result ready")).toBeInTheDocument();

    getAgentRunStatusMock.mockClear();
    await act(async () => {
      await new Promise((resolve) => window.setTimeout(resolve, 2200));
    });
    expect(getAgentRunStatusMock).not.toHaveBeenCalled();
  }, 10000);
});
