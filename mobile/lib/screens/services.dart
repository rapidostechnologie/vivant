import 'package:flutter/material.dart';

import '../api/vivant_api.dart';
import '../app_state.dart';
import '../theme.dart';
import '../widgets/page.dart';

/// Les services où l'on peut se connecter avec Vivant.
class ServicesScreen extends StatefulWidget {
  const ServicesScreen({super.key});

  @override
  State<ServicesScreen> createState() => _ServicesScreenState();
}

class _ServicesScreenState extends State<ServicesScreen> {
  late Future<List<Map<String, dynamic>>> _services = AppScope.read(context).api.services();

  @override
  Widget build(BuildContext context) {
    return RefreshIndicator(
      onRefresh: () async {
        setState(() => _services = AppScope.read(context).api.services());
        await _services;
      },
      child: FutureBuilder(
        future: _services,
        builder: (context, snap) {
          final list = snap.data ?? const [];
          return SubPage(
            title: 'Services',
            back: false,
            children: [
              if (snap.connectionState != ConnectionState.done)
                const Center(child: Padding(padding: EdgeInsets.all(40), child: CircularProgressIndicator()))
              else if (snap.hasError)
                Text(snap.error is ApiException ? snap.error.toString() : 'Une erreur est survenue.',
                    style: VText.subtitle)
              else if (list.isEmpty)
                const Text('Bientôt disponibles.', style: VText.subtitle)
              else
                RowGroup(children: [
                  for (final s in list)
                    ListTile(
                      minTileHeight: 64,
                      leading: Container(
                        width: 40,
                        height: 40,
                        alignment: Alignment.center,
                        decoration: BoxDecoration(color: VColors.greenSoft, borderRadius: BorderRadius.circular(12)),
                        child: Text((s['name'] as String).characters.first.toUpperCase(),
                            style: const TextStyle(fontWeight: FontWeight.w800, color: VColors.greenText)),
                      ),
                      title: Text(s['name'] as String, style: const TextStyle(fontSize: 16, fontWeight: FontWeight.w600)),
                      subtitle: s['sector'] == null || (s['sector'] as String).isEmpty
                          ? null
                          : Text(s['sector'] as String, style: const TextStyle(color: VColors.muted)),
                    ),
                ]),
            ],
          );
        },
      ),
    );
  }
}
