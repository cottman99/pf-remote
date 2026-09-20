using System.Collections.ObjectModel;

namespace PFRemoteCenter.Presentation;

internal sealed class CatalogInteractionState
{
    internal bool IsUnavailable { get; private set; }
    internal string CatalogMessage { get; set; } = "";
    internal string? OperationMessage { get; set; }
    internal string Message => OperationMessage ?? CatalogMessage;
    internal void RecordSuccess() => IsUnavailable = false;
    internal void RecordFailure() => IsUnavailable = true;
}

internal static class CollectionReconciler
{
    // Preserve unaffected item instances and emit individual changes, never Reset.
    internal static void Update<T, TKey>(ObservableCollection<T> current,
        IReadOnlyList<T> next, Func<T, TKey> key, Func<T, T, bool> equal)
    {
        for (int index = 0; index < next.Count; index++)
        {
            int found = index;
            while (found < current.Count && !EqualityComparer<TKey>.Default.Equals(key(current[found]), key(next[index])))
                found++;
            if (found == current.Count) current.Insert(index, next[index]);
            else
            {
                if (found != index) current.Move(found, index);
                if (!equal(current[index], next[index])) current[index] = next[index];
            }
        }
        while (current.Count > next.Count) current.RemoveAt(current.Count - 1);
    }
}
