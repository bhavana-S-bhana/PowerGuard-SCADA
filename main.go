package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"power-plant-scada/database"
	"power-plant-scada/handlers"
	"power-plant-scada/telemetry"
	"power-plant-scada/websocket"

	webview2 "github.com/jchv/go-webview2"
)

func main() {
	port := flag.Int("port", 0, "HTTP SCADA Server Port (0 for dynamic loopback port)")
	dbPath := flag.String("db", "scada_control.db", "SQLite Database File Path")
	flag.Parse()

	log.Println("=========================================================================")
	log.Println("     POWER PLANT SCADA CONTROL SYSTEM - NATIVE DESKTOP AUTOMATION ENGINE ")
	log.Println("=========================================================================")

	// 1. Initialize Embedded SQLite Database
	db, err := database.InitDB(*dbPath)
	if err != nil {
		log.Fatalf("[FATAL] Database initialization failure: %v", err)
	}
	defer db.Close()

	// 2. Initialize WebSocket Hub
	hub := websocket.NewHub()
	go hub.Run()

	// 3. Initialize Telemetry Monitoring & Simulation Goroutines
	monitor := telemetry.NewMonitor(db, hub, 500*time.Millisecond)
	monitor.Start()
	defer monitor.Stop()

	// 4. Initialize REST API
	api := handlers.NewAPI(db, hub, monitor)

	// 5. Configure Router & Endpoints
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		hub.ServeWS(w, r, func() interface{} {
			tags, _ := db.GetAllTags()
			alarms, _ := db.GetActiveAlarms()
			return map[string]interface{}{
				"tags":          tags,
				"active_alarms": alarms,
				"sim_active":    monitor.IsSimActive(),
			}
		})
	})

	// Serve Embedded Static Web UI
	staticFS := getStaticFileSystem()
	fileServer := http.FileServer(staticFS)
	mux.Handle("/", fileServer)

	// 6. Create Net Listener (loopback interface to avoid port conflict)
	listenAddr := fmt.Sprintf("127.0.0.1:%d", *port)
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Fatalf("[FATAL] Failed to bind local TCP listener: %v", err)
	}
	defer listener.Close()

	actualAddr := listener.Addr().String()
	log.Printf("[SCADA SERVER] Internal HTTP Control Engine live at: http://%s", actualAddr)
	log.Printf("[SCADA SERVER] Real-time WebSocket hub active at: ws://%s/ws", actualAddr)

	srv := &http.Server{
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Launch HTTP Server in background Goroutine
	go func() {
		if err := srv.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Printf("[SCADA SERVER] Server listen error: %v", err)
		}
	}()

	// 7. Launch Native Desktop Window (go-webview2)
	w := webview2.New(true)
	if w == nil {
		log.Fatalf("[FATAL] Failed to initialize native WebView2 desktop window.")
	}
	defer w.Destroy()

	w.SetTitle("POWER PLANT SCADA CONTROL SYSTEM - 24/7 AUTOMATION ENGINE")
	w.SetSize(1400, 900, webview2.HintNone)
	w.Navigate("http://" + actualAddr)

	// Run Native OS Desktop Event Loop (blocks until user closes window)
	w.Run()

	log.Println("\n[SCADA SHUTDOWN] Native desktop window closed. Initiating shutdown...")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	log.Println("[SCADA SHUTDOWN] System safely terminated.")
}
