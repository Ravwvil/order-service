document.addEventListener('DOMContentLoaded', () => {
    const orderIdInput = document.getElementById('orderIdInput');
    const getOrderBtn = document.getElementById('getOrderBtn');
    const orderDetailsContainer = document.getElementById('orderDetails');
    const errorMessageContainer = document.getElementById('error-message');

    getOrderBtn.addEventListener('click', fetchOrder);
    orderIdInput.addEventListener('keypress', (e) => {
        if (e.key === 'Enter') {
            fetchOrder();
        }
    });

    async function fetchOrder() {
        const orderId = orderIdInput.value.trim();
        if (!orderId) {
            showError('Please enter an Order UID.');
            return;
        }

        clearDetails();

        try {
            // Using a relative URL, which will be proxied by Nginx
            const response = await fetch(`/api/order/${orderId}`);

            if (!response.ok) {
                const errorData = await response.text();
                throw new Error(`Order not found or error fetching data. Status: ${response.status}. Details: ${errorData}`);
            }

            const orderData = await response.json();
            displayOrder(orderData);

        } catch (error) {
            console.error('Fetch error:', error);
            showError(error.message);
        }
    }

    function displayOrder(data) {
        const formattedJson = JSON.stringify(data, null, 2);
        orderDetailsContainer.innerHTML = `<pre>${formattedJson}</pre>`;
    }

    function showError(message) {
        errorMessageContainer.textContent = message;
        errorMessageContainer.style.display = 'block';
    }

    function clearDetails() {
        orderDetailsContainer.innerHTML = '';
        errorMessageContainer.textContent = '';
        errorMessageContainer.style.display = 'none';
    }
}); 