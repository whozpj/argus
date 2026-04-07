#!/usr/bin/env python3
"""
Argus demo app — a minimal Q&A assistant instrumented with argus-sdk.

Usage:
    # With Anthropic
    ANTHROPIC_API_KEY=sk-... python app.py

    # With OpenAI
    OPENAI_API_KEY=sk-... python app.py --provider openai

    # Point at a non-default Argus server
    python app.py --argus http://my-server:4000
"""

import argparse
import os
import sys


def parse_args():
    p = argparse.ArgumentParser(description="Argus demo Q&A assistant")
    p.add_argument("--provider", choices=["anthropic", "openai"], default="anthropic")
    p.add_argument("--argus", default="http://localhost:4000", metavar="URL",
                   help="Argus server endpoint (default: http://localhost:4000)")
    p.add_argument("--model", default=None,
                   help="Model to use (default: claude-sonnet-4-6 or gpt-4o)")
    return p.parse_args()


def main():
    args = parse_args()

    # ── 1. Patch all clients before importing the provider library ────────────
    try:
        from argus_sdk import patch
        patch(endpoint=args.argus)
        print(f"[argus] instrumented — sending signals to {args.argus}")
    except ImportError:
        print("[argus] argus-sdk not installed — signals will not be sent")
        print("        Run: pip install argus-sdk  or  pip install -e ../../sdk")

    # ── 2. Build the provider client ──────────────────────────────────────────
    if args.provider == "anthropic":
        try:
            import anthropic
        except ImportError:
            sys.exit("anthropic package not installed. Run: pip install anthropic")

        api_key = os.environ.get("ANTHROPIC_API_KEY")
        if not api_key:
            sys.exit("ANTHROPIC_API_KEY environment variable not set.")

        client = anthropic.Anthropic(api_key=api_key)
        model = args.model or "claude-sonnet-4-6"

        def ask(question: str) -> str:
            response = client.messages.create(
                model=model,
                max_tokens=512,
                messages=[{"role": "user", "content": question}],
            )
            return response.content[0].text

    else:
        try:
            import openai
        except ImportError:
            sys.exit("openai package not installed. Run: pip install openai")

        api_key = os.environ.get("OPENAI_API_KEY")
        if not api_key:
            sys.exit("OPENAI_API_KEY environment variable not set.")

        client = openai.OpenAI(api_key=api_key)
        model = args.model or "gpt-4o"

        def ask(question: str) -> str:
            response = client.chat.completions.create(
                model=model,
                max_tokens=512,
                messages=[{"role": "user", "content": question}],
            )
            return response.choices[0].message.content

    # ── 3. Interactive Q&A loop ───────────────────────────────────────────────
    print(f"\nUsing {args.provider} / {model}")
    print("Type a question and press Enter. Ctrl-C or 'quit' to exit.\n")

    while True:
        try:
            question = input("You: ").strip()
        except (KeyboardInterrupt, EOFError):
            print("\nBye.")
            break

        if not question or question.lower() in ("quit", "exit", "q"):
            print("Bye.")
            break

        try:
            answer = ask(question)
            print(f"\nAssistant: {answer}\n")
        except Exception as e:
            print(f"[error] {e}\n")


if __name__ == "__main__":
    main()
