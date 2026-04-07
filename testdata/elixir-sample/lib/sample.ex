defmodule Sample do
  @moduledoc """
  Mixed used and unused code so we can see exactly what mix compile and
  mix xref report on Elixir 1.19.
  """

  # Used: called from main/0 below.
  def hello(name) do
    "hello, #{name}"
  end

  def main do
    IO.puts(hello("world"))
    case classify(:ok) do
      :good -> IO.puts("good")
      :bad -> IO.puts("bad")
    end
  end

  # The type system should flag the :error clause: classify is only ever
  # called with :ok in this module, so the :error clause is unreachable.
  defp classify(:ok), do: :good
  defp classify(:error), do: :bad

  # Unreachable case clause: tuple shape can never match a bare atom.
  def impossible_match(value) do
    case value do
      {:ok, _} -> :ok
      :ok -> :ok  # actually reachable
      {:error, _, _, _} when is_tuple(value) and tuple_size(value) == 2 -> :unreachable
    end
  end
end
