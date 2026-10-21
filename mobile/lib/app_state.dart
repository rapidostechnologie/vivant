import 'package:flutter/widgets.dart';

import 'api/vivant_api.dart';
import 'storage/device_store.dart';

/// État global de l'application : le téléphone est-il inscrit, et déverrouillé ?
class AppState extends ChangeNotifier {
  AppState({VivantApi? api, DeviceStore? store})
      : api = api ?? VivantApi(),
        store = store ?? DeviceStore();

  final VivantApi api;
  final DeviceStore store;

  DeviceIdentity? identity;
  bool unlocked = false;

  Future<void> load() async {
    identity = await store.load();
    notifyListeners();
  }

  Future<void> enrolled(DeviceIdentity id, String pin) async {
    await store.save(id, pin);
    identity = id;
    unlocked = true;
    notifyListeners();
  }

  void setUnlocked(bool value) {
    if (unlocked == value) return;
    unlocked = value;
    notifyListeners();
  }

  /// Supprime l'identité du téléphone (le serveur la révoque d'abord).
  Future<void> forget() async {
    final id = identity;
    if (id != null) {
      try {
        await api.revoke(id);
      } on ApiException catch (e) {
        if (e.status != 401) rethrow; // déjà révoqué : on efface quand même
      }
    }
    await store.clear();
    identity = null;
    unlocked = false;
    notifyListeners();
  }
}

/// Donne accès à [AppState] dans l'arbre des widgets.
class AppScope extends InheritedNotifier<AppState> {
  const AppScope({super.key, required AppState state, required super.child}) : super(notifier: state);

  static AppState of(BuildContext context) => context.dependOnInheritedWidgetOfExactType<AppScope>()!.notifier!;

  /// Sans s'abonner aux changements (pour les actions).
  static AppState read(BuildContext context) => context.getInheritedWidgetOfExactType<AppScope>()!.notifier!;
}
