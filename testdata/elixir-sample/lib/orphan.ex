defmodule Sample.Orphan do
  @moduledoc """
  An entire module that is never referenced from anywhere. mix xref
  unreachable should pick this up — and so should mix_unused if it
  were enabled.
  """

  def never_called do
    :ok
  end

  def also_never_called(_x) do
    :ok
  end

  defp private_dead do
    :ok
  end
end
