defmodule Sample.FakeController do
  @moduledoc """
  Stand-in for a Phoenix controller. The default-ignore filter in our
  Elixir adapter should suppress these — they look unused but in a real
  Phoenix app the router dispatches to them.
  """

  def index(_conn, _params), do: :ok
  def show(_conn, _params), do: :ok
  def create(_conn, _params), do: :ok
end
