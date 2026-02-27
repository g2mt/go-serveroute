document.addEventListener('DOMContentLoaded', () => {
    const servicesBody = document.getElementById('services-body');
    const scheme = window.location.protocol;
    const host = window.location.host;
    const apiBase = `${scheme}//api.${host}`;

    // Fetch initial services list
    fetch(`${apiBase}/list`)
        .then(response => response.json())
        .then(services => {
            renderServices(services);
            setupEventSource();
        })
        .catch(error => {
            console.error('Error fetching services:', error);
        });

    function renderServices(services) {
        servicesBody.innerHTML = '';
        Object.entries(services).forEach(([name, service]) => {
            const row = document.createElement('tr');
            row.dataset.service = name;
            
            // Create status cell
            const statusCell = document.createElement('td');
            statusCell.className = `status ${service.status}`;
            statusCell.textContent = service.status;
            
            // Create name cell
            const nameCell = document.createElement('td');
            nameCell.textContent = name;
            
            // Create actions cell
            const actionsCell = document.createElement('td');
            actionsCell.className = 'actions';
            
            // Create start button
            const startButton = document.createElement('button');
            startButton.className = 'start-btn';
            startButton.textContent = 'Start';
            startButton.dataset.name = name;
            if (service.status === 'started') {
                startButton.disabled = true;
            }
            
            // Create stop button
            const stopButton = document.createElement('button');
            stopButton.className = 'stop-btn';
            stopButton.textContent = 'Stop';
            stopButton.dataset.name = name;
            if (service.status === 'stopped') {
                stopButton.disabled = true;
            }
            
            // Assemble the row
            actionsCell.appendChild(startButton);
            actionsCell.appendChild(stopButton);
            row.appendChild(statusCell);
            row.appendChild(nameCell);
            row.appendChild(actionsCell);
            
            servicesBody.appendChild(row);
        });

        // Add event listeners to buttons
        document.querySelectorAll('.start-btn').forEach(button => {
            button.addEventListener('click', () => {
                const name = button.dataset.name;
                fetch(`${apiBase}/start`, {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json'
                    },
                    body: JSON.stringify({ service: name })
                });
            });
        });

        document.querySelectorAll('.stop-btn').forEach(button => {
            button.addEventListener('click', () => {
                const name = button.dataset.name;
                fetch(`${apiBase}/stop`, {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json'
                    },
                    body: JSON.stringify({ service: name })
                });
            });
        });
    }

    function updateServiceStatus(serviceName, status) {
        const row = document.querySelector(`tr[data-service="${serviceName}"]`);
        if (row) {
            const statusCell = row.querySelector('.status');
            statusCell.className = `status ${status}`;
            statusCell.textContent = status;
            
            const startButton = row.querySelector('.start-btn');
            const stopButton = row.querySelector('.stop-btn');
            
            if (status === 'started') {
                startButton.disabled = true;
                stopButton.disabled = false;
            } else if (status === 'stopped') {
                startButton.disabled = false;
                stopButton.disabled = true;
            }
        }
    }

    function setupEventSource() {
        const eventSource = new EventSource(`${apiBase}/events`);
        
        eventSource.addEventListener('connected', (event) => {
            console.log('Connected to event stream');
        });

        eventSource.addEventListener('message', (event) => {
            const eventData = JSON.parse(event.data);
            console.log(eventData);
            if (eventData.type === 'start') {
                updateServiceStatus(eventData.service, 'started');
            } else if (eventData.type === 'stop') {
                updateServiceStatus(eventData.service, 'stopped');
            }
        });

        eventSource.addEventListener('error', (error) => {
            console.error('EventSource failed:', error);
        });
    }
});
